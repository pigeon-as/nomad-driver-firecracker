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
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/machine"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/utils"
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
	ReattachConfig *structs.ReattachConfig
	TaskConfig     *drivers.TaskConfig
	StartedAt      time.Time
	Pid            int
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

// deriveSocketPath computes the Firecracker socket path from task directory and ID.
// The path is deterministic and can be derived from task config during recovery.
func deriveSocketPath(taskDir, taskID string) string {
	return filepath.Join(taskDir, "jailer", taskID, "root", "run", "firecracker.socket")
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
	defer ticker.Stop()
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

func (d *FirecrackerDriverPlugin) StartTask(cfg *drivers.TaskConfig) (handle *drivers.TaskHandle, network *drivers.DriverNetwork, err error) {
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

	paths, err := jailer.BuildPaths(cfg.TaskDir().Dir, cfg.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create jailer paths: %v", err)
	}

	configPath := paths.ConfigPathHost
	configPathChroot := paths.ConfigPathChroot
	logPathChroot := paths.LogPathChroot
	vmCfg := &machine.Config{
		BootSource:        driverConfig.BootSource,
		Drives:            driverConfig.Drives,
		NetworkInterfaces: driverConfig.NetworkInterfaces,
	}
	_, err = machine.BuildVMConfig(configPath, vmCfg, cfg.Resources)
	if err != nil {
		// Cleanup only vmconfig.json on failure; don't remove the config directory
		// as it was created by BuildPaths and may be needed by the jailer
		_ = os.Remove(configPath)
		return nil, nil, fmt.Errorf("failed to build vm configuration: %v", err)
	}
	d.logger.Debug("generated vm configuration", "path", configPath)

	d.logger.Info("starting task", "driver_cfg", hclog.Fmt("%+v", driverConfig))
	if len(driverConfig.NetworkInterfaces) > 0 {
		d.logger.Debug("network configuration", "network", driverConfig.NetworkInterfaces)
	}
	handle = drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	executorConfig := &executor.ExecutorConfig{
		LogFile:  filepath.Join(cfg.TaskDir().Dir, "executor.out"),
		LogLevel: "debug",
	}

	exec, pluginClient, err := executor.CreateExecutor(d.logger, d.nomadConfig, executorConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create executor: %v", err)
	}

	// Guarantee cleanup of executor resources on any error
	defer func() {
		if err != nil {
			pluginClient.Kill()
			if shutdownErr := exec.Shutdown("", 0); shutdownErr != nil {
				d.logger.Error("failed to shutdown executor on error", "error", shutdownErr)
			}
		}
	}()

	if d.config == nil || d.config.Jailer == nil {
		err = errors.New("jailer configuration missing")
		return nil, nil, err
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

	jArgs, err := jConfig.BuildArgs(cfg.TaskDir().Dir, params, "--config-file", configPathChroot, "--log-path", logPathChroot)
	if err != nil {
		err = fmt.Errorf("invalid jailer configuration: %v", err)
		return nil, nil, err
	}
	execCmd := &executor.ExecCommand{
		Cmd:        jConfig.Bin(),
		Args:       jArgs,
		StdoutPath: cfg.StdoutPath,
		StderrPath: cfg.StderrPath,
	}

	ps, err := exec.Launch(execCmd)
	if err != nil {
		err = fmt.Errorf("failed to launch command with executor: %v", err)
		return nil, nil, err
	}

	d.logger.Info("firecracker process launched", "task_id", cfg.ID, "pid", ps.Pid)

	// Give Firecracker time to create socket and be ready for API calls
	// Firecracker docs recommend 15-30ms before configuration calls
	time.Sleep(30 * time.Millisecond)

	// Verify socket is accessible before returning handle
	socketPath := deriveSocketPath(cfg.TaskDir().Dir, cfg.ID)
	if err := d.waitForSocket(socketPath, 5*time.Second); err != nil {
		d.logger.Warn("socket not ready after startup", "task_id", cfg.ID, "err", err)
	}

	d.logger.Debug("firecracker socket ready", "task_id", cfg.ID, "socket_path", socketPath)

	h := &taskHandle{
		exec:         exec,
		pid:          ps.Pid,
		pluginClient: pluginClient,
		taskConfig:   cfg,
		procState:    drivers.TaskStateRunning,
		startedAt:    time.Now().Round(time.Millisecond),
		logger:       d.logger,
		socketPath:   socketPath,
	}

	driverState := TaskState{
		ReattachConfig: structs.ReattachConfigFromGoPlugin(pluginClient.ReattachConfig()),
		Pid:            ps.Pid,
		TaskConfig:     cfg,
		StartedAt:      h.startedAt,
	}

	if err = handle.SetDriverState(&driverState); err != nil {
		err = fmt.Errorf("failed to set driver state: %v", err)
		return nil, nil, err
	}

	d.tasks.Set(cfg.ID, h)
	go h.run()
	d.logger.Info("task started successfully", "task_id", cfg.ID)

	// Build network information from configured interfaces
	var driverNetwork *drivers.DriverNetwork
	if len(driverConfig.NetworkInterfaces) > 0 {
		driverNetwork = &drivers.DriverNetwork{
			PortMap: map[string]int{},
		}
	}

	return handle, driverNetwork, nil
}

// waitForSocket verifies that Firecracker socket is accessible and API is responding.
// Follows official Firecracker SDK pattern: first check socket file exists, then verify
// API responds to a health check. Uses tight 10ms polling interval for quick detection.
func (d *FirecrackerDriverPlugin) waitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Step 1: Verify socket file exists (required for API connectivity)
			if _, err := os.Stat(socketPath); err != nil {
				if time.Now().After(deadline) {
					return fmt.Errorf("socket file not found after %v: %v", timeout, err)
				}
				continue
			}

			// Step 2: Verify API responds (health check)
			c := client.New(socketPath)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			_, err := c.GetInstanceInfo(ctx)
			cancel()

			if err == nil {
				return nil // Socket ready: file exists and API responds
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("socket not ready after %v: %v", timeout, err)
			}
		case <-d.ctx.Done():
			return fmt.Errorf("socket verification cancelled")
		}
	}
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
	d.logger.Info("recovering task", "task_id", handle.Config.ID, "pid", taskState.Pid)
	var driverConfig TaskConfig
	if err := taskState.TaskConfig.DecodeDriverConfig(&driverConfig); err != nil {
		return fmt.Errorf("failed to decode driver config: %v", err)
	}

	plugRC, err := structs.ReattachConfigToGoPlugin(taskState.ReattachConfig)
	if err != nil {
		return fmt.Errorf("failed to build ReattachConfig from taskConfig state: %v", err)
	}

	execImpl, pluginClient, err := executor.ReattachToExecutor(plugRC, d.logger, d.nomadConfig.Topology.Compute())
	if err != nil {
		return fmt.Errorf("failed to reattach to executor: %v", err)
	}

	socketPath := deriveSocketPath(taskState.TaskConfig.TaskDir().Dir, taskState.TaskConfig.ID)
	if err := d.waitForSocket(socketPath, 5*time.Second); err != nil {
		d.logger.Warn("socket not ready after recovery", "task_id", taskState.TaskConfig.ID, "err", err)
		socketPath = ""
	}

	h := &taskHandle{
		exec:         execImpl,
		pid:          taskState.Pid,
		pluginClient: pluginClient,
		taskConfig:   taskState.TaskConfig,
		procState:    drivers.TaskStateRunning,
		startedAt:    taskState.StartedAt,
		exitResult:   &drivers.ExitResult{},
		logger:       d.logger,
		socketPath:   socketPath,
	}

	d.tasks.Set(taskState.TaskConfig.ID, h)

	go h.run()
	d.logger.Info("task recovered successfully", "task_id", taskState.TaskConfig.ID)
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

	// For SIGTERM/SIGINT, attempt graceful VM shutdown via Ctrl+Alt+Del first
	// This matches QEMU pattern: try graceful shutdown, then let executor handle fallback
	if signal == "SIGTERM" || signal == "SIGINT" {
		// Non-blocking attempt at graceful shutdown; returns immediately
		_ = handle.forwardSignal(context.Background(), signal, 5*time.Second)
		// Fall through to executor shutdown with full timeout for enforcement
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

	if !handle.pluginClient.Exited() {
		if err := handle.exec.Shutdown("", 0); err != nil {
			handle.logger.Error("destroying executor failed", "err", err)
		}

		handle.pluginClient.Kill()
	}

	d.tasks.Delete(taskID)

	// Clean up jailer directory structure
	if handle.taskConfig != nil && handle.taskConfig.TaskDir() != nil {
		jailorPath := filepath.Join(handle.taskConfig.TaskDir().Dir, "jailer", handle.taskConfig.ID)
		if err := os.RemoveAll(jailorPath); err != nil {
			handle.logger.Warn("failed to clean up jailer directory", "path", jailorPath, "err", err)
		}
	}

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

// SignalTask forwards a signal to the Firecracker VMM process.
// SIGTERM and SIGINT trigger graceful VM shutdown via Ctrl+Alt+Del if available,
// otherwise the signal is forwarded to the executor process.
// All other signals are forwarded to the Firecracker process.
func (d *FirecrackerDriverPlugin) SignalTask(taskID string, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	return handle.forwardSignal(context.Background(), signal, 30*time.Second)
}

func (d *FirecrackerDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	return nil, errors.New("exec is not supported for Firecracker VMs; configure your guest OS to handle command execution externally")
}
