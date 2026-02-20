// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/drivers/shared/eventer"
	"github.com/hashicorp/nomad/drivers/shared/executor"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"github.com/hashicorp/nomad/plugins/shared/structs"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/client"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/vm"
)

const (
	pluginName = "firecracker"

	pluginVersion = "v0.0.1"

	fingerprintPeriod = 30 * time.Second

	taskHandleVersion = 1
)

var (
	pluginInfo = &base.PluginInfoResponse{
		Type:              base.PluginTypeDriver,
		PluginApiVersions: []string{drivers.ApiVersion010},
		PluginVersion:     pluginVersion,
		Name:              pluginName,
	}
)

type TaskState struct {
	ReattachConfig  *structs.ReattachConfig
	TaskConfig      *drivers.TaskConfig
	StartedAt       time.Time
	Pid             int
	SocketPath      string
	SnapshotMemPath string
	SnapshotPath    string
}

type FirecrackerDriverPlugin struct {
	eventer        *eventer.Eventer
	config         *Config
	nomadConfig    *base.ClientDriverConfig
	tasks          *taskStore
	ctx            context.Context
	signalShutdown context.CancelFunc
	logger         hclog.Logger
}

func NewPlugin(logger hclog.Logger) drivers.DriverPlugin {
	ctx, cancel := context.WithCancel(context.Background())
	logger = logger.Named(pluginName)

	return &FirecrackerDriverPlugin{
		eventer:        eventer.NewEventer(ctx, logger),
		config:         &Config{},
		tasks:          newTaskStore(),
		ctx:            ctx,
		signalShutdown: cancel,
		logger:         logger,
	}
}

func (d *FirecrackerDriverPlugin) PluginInfo() (*base.PluginInfoResponse, error) {
	return pluginInfo, nil
}

func (d *FirecrackerDriverPlugin) ConfigSchema() (*hclspec.Spec, error) {
	return configSpec, nil
}

func (d *FirecrackerDriverPlugin) SetConfig(cfg *base.Config) error {
	var config Config
	if len(cfg.PluginConfig) != 0 {
		if err := base.MsgPackDecode(cfg.PluginConfig, &config); err != nil {
			return err
		}
	}

	d.config = &config

	if err := d.config.Validate(); err != nil {
		return err
	}

	if cfg.AgentConfig != nil {
		d.nomadConfig = cfg.AgentConfig.Driver
	}

	return nil
}

func (d *FirecrackerDriverPlugin) TaskConfigSchema() (*hclspec.Spec, error) {
	return taskConfigSpec, nil
}

func (d *FirecrackerDriverPlugin) Capabilities() (*drivers.Capabilities, error) {
	return capabilities, nil
}

func (d *FirecrackerDriverPlugin) Fingerprint(ctx context.Context) (<-chan *drivers.Fingerprint, error) {
	ch := make(chan *drivers.Fingerprint)
	go d.handleFingerprint(ctx, ch)
	return ch, nil
}

func (d *FirecrackerDriverPlugin) handleFingerprint(ctx context.Context, ch chan<- *drivers.Fingerprint) {
	defer close(ch)
	ticker := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(fingerprintPeriod)
			ch <- d.buildFingerprint()
		}
	}
}

func (d *FirecrackerDriverPlugin) buildFingerprint() *drivers.Fingerprint {
	fp := &drivers.Fingerprint{
		Attributes:        map[string]*structs.Attribute{},
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}

	if d.config == nil || d.config.Jailer == nil || d.config.Jailer.ExecFile == "" {
		return fp
	}

	bin := d.config.Jailer.ExecFile
	binPath, err := exec.LookPath(bin)
	if err != nil {
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = fmt.Sprintf("firecracker binary %s not found: %v", bin, err)
		return fp
	}

	version := utils.QueryVersion(binPath)
	if version != "" {
		fp.Attributes["driver.firecracker.version"] = structs.NewStringAttribute(version)
	}

	return fp
}

func (d *FirecrackerDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to decode driver config: %v", err)
	}

	if err := driverConfig.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid task configuration: %v", err)
	}

	configPath := filepath.Join(cfg.TaskDir().Dir, "vmconfig.json")
	vmCfg := &vm.Config{
		BootSource:        driverConfig.BootSource,
		Drives:            driverConfig.Drives,
		NetworkInterfaces: driverConfig.NetworkInterfaces,
	}
	jsonData, err := vm.BuildVMConfig(configPath, vmCfg, cfg.Resources)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build vm configuration: %v", err)
	}
	d.logger.Debug("generated vm configuration", "path", configPath, "json", string(jsonData))

	d.logger.Info("starting task", "driver_cfg", hclog.Fmt("%+v", driverConfig))
	if len(driverConfig.NetworkInterfaces) > 0 {
		d.logger.Debug("network configuration", "network", driverConfig.NetworkInterfaces)
	}
	handle := drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	executorConfig := &executor.ExecutorConfig{
		LogFile:  filepath.Join(cfg.TaskDir().Dir, "executor.out"),
		LogLevel: "debug",
	}

	exec, pluginClient, err := executor.CreateExecutor(d.logger, d.nomadConfig, executorConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create executor: %v", err)
	}

	if d.config == nil || d.config.Jailer == nil {
		pluginClient.Kill()
		return nil, nil, errors.New("jailer configuration missing")
	}

	jConfig := d.config.Jailer
	params := &jailer.BuildParams{
		ID: cfg.ID,
	}

	if cfg.User != "" {
		if uid, gid, err := jailer.ResolveUserIDs(cfg.User); err != nil {
			d.logger.Warn("failed to resolve task user for jailer", "user", cfg.User, "err", err)
		} else {
			params.UID = uid
			params.GID = gid
		}
	}

	if cfg.NetworkIsolation != nil && cfg.NetworkIsolation.Path != "" {
		params.NetNS = cfg.NetworkIsolation.Path
	}

	jArgs, err := jConfig.BuildArgs(cfg.TaskDir().Dir, params, "--config-file", configPath, "--log-path", cfg.StderrPath)
	if err != nil {
		pluginClient.Kill()
		return nil, nil, fmt.Errorf("invalid jailer configuration: %v", err)
	}
	execCmd := &executor.ExecCommand{
		Cmd:        jConfig.Bin(),
		Args:       jArgs,
		StdoutPath: cfg.StdoutPath,
		StderrPath: cfg.StderrPath,
	}

	ps, err := exec.Launch(execCmd)
	if err != nil {
		pluginClient.Kill()
		return nil, nil, fmt.Errorf("failed to launch command with executor: %v", err)
	}

	h := &taskHandle{
		exec:         exec,
		pid:          ps.Pid,
		pluginClient: pluginClient,
		taskConfig:   cfg,
		procState:    drivers.TaskStateRunning,
		startedAt:    time.Now().Round(time.Millisecond),
		logger:       d.logger,
		socketPath:   filepath.Join(cfg.TaskDir().Dir, "jailer", cfg.ID, "root", "run", "firecracker.socket"),
	}

	driverState := TaskState{
		ReattachConfig: structs.ReattachConfigFromGoPlugin(pluginClient.ReattachConfig()),
		Pid:            ps.Pid,
		TaskConfig:     cfg,
		StartedAt:      h.startedAt,
		SocketPath:     h.socketPath,
	}

	if err := handle.SetDriverState(&driverState); err != nil {
		pluginClient.Kill()
		if shutdownErr := exec.Shutdown("", 0); shutdownErr != nil {
			d.logger.Error("failed to shutdown executor after SetDriverState error", "error", shutdownErr)
		}
		return nil, nil, fmt.Errorf("failed to set driver state: %v", err)
	}

	d.tasks.Set(cfg.ID, h)
	go h.run()
	return handle, nil, nil
}

func (d *FirecrackerDriverPlugin) RecoverTask(handle *drivers.TaskHandle) error {
	if handle == nil {
		return errors.New("handle cannot be nil")
	}

	if _, ok := d.tasks.Get(handle.Config.ID); ok {
		return nil
	}

	var taskState TaskState
	if err := handle.GetDriverState(&taskState); err != nil {
		return fmt.Errorf("failed to decode task state from handle: %v", err)
	}

	var driverConfig TaskConfig
	if err := taskState.TaskConfig.DecodeDriverConfig(&driverConfig); err != nil {
		return fmt.Errorf("failed to decode driver config: %v", err)
	}

	if len(driverConfig.NetworkInterfaces) > 0 {
		if err := driverConfig.NetworkInterfaces.Validate(); err != nil {
			return fmt.Errorf("invalid network configuration on recover: %v", err)
		}
	}

	plugRC, err := structs.ReattachConfigToGoPlugin(taskState.ReattachConfig)
	if err != nil {
		return fmt.Errorf("failed to build ReattachConfig from taskConfig state: %v", err)
	}

	execImpl, pluginClient, err := executor.ReattachToExecutor(plugRC, d.logger, d.nomadConfig.Topology.Compute())
	if err != nil {
		return fmt.Errorf("failed to reattach to executor: %v", err)
	}

	h := &taskHandle{
		exec:            execImpl,
		pid:             taskState.Pid,
		pluginClient:    pluginClient,
		taskConfig:      taskState.TaskConfig,
		procState:       drivers.TaskStateRunning,
		startedAt:       taskState.StartedAt,
		exitResult:      &drivers.ExitResult{},
		socketPath:      taskState.SocketPath,
		logger:          d.logger,
		snapshotMemPath: taskState.SnapshotMemPath,
		snapshotPath:    taskState.SnapshotPath,
	}

	if h.socketPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		c := client.New(h.socketPath)
		info, err := c.GetInstanceInfo(ctx)
		if err != nil {
			return fmt.Errorf("recovered VM failed health check at socket %s: %v", h.socketPath, err)
		}
		if info != nil {
			d.logger.Debug("recovered VM is responsive", "task_id", h.taskConfig.ID)
		}
	}

	d.tasks.Set(taskState.TaskConfig.ID, h)

	go h.run()
	return nil
}

func (d *FirecrackerDriverPlugin) WaitTask(ctx context.Context, taskID string) (<-chan *drivers.ExitResult, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	ch := make(chan *drivers.ExitResult)
	go d.handleWait(ctx, handle, ch)
	return ch, nil
}

func (d *FirecrackerDriverPlugin) handleWait(ctx context.Context, handle *taskHandle, ch chan *drivers.ExitResult) {
	defer close(ch)
	var result *drivers.ExitResult

	ps, err := handle.exec.Wait(ctx)
	if err != nil {
		result = &drivers.ExitResult{
			Err: fmt.Errorf("executor: error waiting on process: %v", err),
		}
	} else {
		result = &drivers.ExitResult{
			ExitCode: ps.ExitCode,
			Signal:   ps.Signal,
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case ch <- result:
		}
	}
}

func (d *FirecrackerDriverPlugin) StopTask(taskID string, timeout time.Duration, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	// Attempt graceful shutdown via Ctrl+Alt+Del for SIGTERM/SIGINT
	if (signal == "SIGTERM" || signal == "SIGINT") && handle.socketPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		c := client.New(handle.socketPath)
		if err := c.SendCtrlAltDel(ctx); err != nil {
			d.logger.Warn("graceful shutdown failed, will force kill", "signal", signal, "err", err)
		} else {
			d.logger.Info("graceful shutdown initiated via Ctrl+Alt+Del", "task_id", taskID)
			return nil
		}
	}

	if err := handle.exec.Shutdown(signal, timeout); err != nil {
		if handle.pluginClient.Exited() {
			return nil
		}
		return fmt.Errorf("executor Shutdown failed: %v", err)
	}

	return nil
}

func (d *FirecrackerDriverPlugin) DestroyTask(taskID string, force bool) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	if handle.IsRunning() && !force {
		return errors.New("cannot destroy running task")
	}

	if handle.IsRunning() && force {
		if handle.socketPath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()

			c := client.New(handle.socketPath)
			if err := c.SendCtrlAltDel(ctx); err != nil {
				d.logger.Debug("graceful shutdown during destroy failed, will force kill", "err", err)
			}
		}
	}

	if !handle.pluginClient.Exited() {
		if err := handle.exec.Shutdown("", 0); err != nil {
			handle.logger.Error("force shutdown failed", "err", err)
		}
		handle.pluginClient.Kill()
	}

	handle.stateLock.RLock()
	memPath := handle.snapshotMemPath
	snapPath := handle.snapshotPath
	handle.stateLock.RUnlock()

	if memPath != "" {
		if err := os.Remove(memPath); err != nil && !os.IsNotExist(err) {
			d.logger.Warn("failed to remove snapshot memory file", "path", memPath, "err", err)
		}
	}
	if snapPath != "" {
		if err := os.Remove(snapPath); err != nil && !os.IsNotExist(err) {
			d.logger.Warn("failed to remove snapshot state file", "path", snapPath, "err", err)
		}
	}

	snapshotDir := filepath.Join(handle.taskConfig.AllocDir, "snapshot")
	if err := os.Remove(snapshotDir); err != nil && !os.IsNotExist(err) {
		d.logger.Debug("snapshot directory not empty or already removed", "path", snapshotDir)
	}

	d.tasks.Delete(taskID)
	return nil
}

func (d *FirecrackerDriverPlugin) InspectTask(taskID string) (*drivers.TaskStatus, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	return handle.TaskStatus(), nil
}

func (d *FirecrackerDriverPlugin) TaskStats(ctx context.Context, taskID string, interval time.Duration) (<-chan *drivers.TaskResourceUsage, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	return handle.exec.Stats(ctx, interval)
}

func (d *FirecrackerDriverPlugin) TaskEvents(ctx context.Context) (<-chan *drivers.TaskEvent, error) {
	return d.eventer.TaskEvents(ctx)
}

func (d *FirecrackerDriverPlugin) SignalTask(taskID string, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	if handle.socketPath == "" {
		d.logger.Warn("cannot send signal: no socket path available", "task_id", taskID)
		return errors.New("socket path not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c := client.New(handle.socketPath)

	switch signal {
	case "SIGTERM", "SIGINT":
		return c.SendCtrlAltDel(ctx)

	case "SIGSTOP":
		if err := c.Pause(ctx); err != nil {
			d.logger.Warn("pause failed", "task_id", taskID, "err", err)
			return fmt.Errorf("pause failed: %v", err)
		}

		snapshotDir := filepath.Join(handle.taskConfig.AllocDir, "snapshot")
		if err := os.MkdirAll(snapshotDir, 0755); err != nil {
			d.logger.Warn("failed to create snapshot directory", "task_id", taskID, "err", err)
			return fmt.Errorf("snapshot dir creation failed: %v", err)
		}

		memPath := filepath.Join(snapshotDir, "memory.img")
		snapPath := filepath.Join(snapshotDir, "state.vmstate")

		if err := c.CreateSnapshot(ctx, memPath, snapPath); err != nil {
			d.logger.Warn("snapshot creation failed, attempting to resume", "task_id", taskID, "err", err)
			if resumeErr := c.Resume(ctx); resumeErr != nil {
				d.logger.Error(
					"resume failed after snapshot error; VM may remain paused and require manual recovery",
					"task_id", taskID,
					"snapshot_err", err,
					"resume_err", resumeErr,
					"recovery_hint", "try sending SIGCONT again if VM becomes accessible, or destroy and restart the task",
				)
				return fmt.Errorf("snapshot creation failed (%v) and resume failed (%v); VM may remain paused - try SIGCONT again or destroy task", err, resumeErr)
			}
			if rmErr := os.RemoveAll(snapshotDir); rmErr != nil {
				d.logger.Warn("failed to clean up snapshot directory after snapshot error", "task_id", taskID, "dir", snapshotDir, "err", rmErr)
			}
			return fmt.Errorf("snapshot creation failed, VM resumed without snapshot: %v", err)
		}

		handle.stateLock.Lock()
		handle.snapshotMemPath = memPath
		handle.snapshotPath = snapPath
		handle.stateLock.Unlock()

		d.logger.Info("VM suspended with snapshot", "task_id", taskID)
		return nil

	case "SIGCONT":
		handle.stateLock.RLock()
		memPath := handle.snapshotMemPath
		snapPath := handle.snapshotPath
		handle.stateLock.RUnlock()

		if memPath == "" || snapPath == "" {
			d.logger.Warn("cannot resume: no snapshot available", "task_id", taskID)
			return errors.New("SIGCONT requires prior SIGSTOP (no snapshot available)")
		}

		// Verify VM is still accessible before attempting resume
		// Note: Firecracker Resume() only works on paused VMs within the same process lifecycle
		// If the VM process was killed or agent restarted, resume will fail
		if _, err := c.GetInstanceInfo(ctx); err != nil {
			d.logger.Warn("cannot resume: VM not accessible", "task_id", taskID, "err", err)
			return fmt.Errorf("VM not accessible for resume (may have been terminated): %v", err)
		}

		if err := c.Resume(ctx); err != nil {
			d.logger.Warn("resume failed", "task_id", taskID, "err", err)
			return fmt.Errorf("resume failed: %v", err)
		}

		d.logger.Info("VM resumed from snapshot", "task_id", taskID)
		return nil

	default:
		return fmt.Errorf("signal not supported: %s", signal)
	}
}

func (d *FirecrackerDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	return nil, errors.New("exec is not supported for Firecracker VMs; configure your guest OS to handle command execution externally")
}
