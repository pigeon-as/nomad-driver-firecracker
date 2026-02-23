// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package firecracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
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
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network_interface"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/snapshot"
)

const (
	pluginName = "firecracker"

	pluginVersion = "v0.0.1"

	fingerprintPeriod = 30 * time.Second

	taskHandleVersion = 1
)

// jailerID returns a globally unique, filesystem-safe identifier for the
// jailer instance. It follows the Docker driver pattern of "taskName-allocID".
func jailerID(cfg *drivers.TaskConfig) string {
	return cfg.Name + "-" + cfg.AllocID
}

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
	eventer     *eventer.Eventer
	config      *Config
	nomadConfig *base.ClientDriverConfig
	tasks       *taskStore
	ctx         context.Context
	logger      hclog.Logger
}

func NewPlugin(ctx context.Context, logger hclog.Logger) drivers.DriverPlugin {
	logger = logger.Named(pluginName)

	return &FirecrackerDriverPlugin{
		eventer: eventer.NewEventer(ctx, logger),
		config:  &Config{},
		tasks:   newTaskStore(),
		ctx:     ctx,
		logger:  logger,
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

func (d *FirecrackerDriverPlugin) prepareGuestFiles(cfg *TaskConfig, configPath, allocDir string) error {
	if d.config == nil {
		return fmt.Errorf("driver configuration not initialized")
	}

	jailerRootDir := filepath.Dir(configPath)

	guestCfg := &jailer.GuestFileConfig{
		Kernel: cfg.BootSource.KernelImagePath,
		Initrd: cfg.BootSource.InitrdPath,
	}

	if len(cfg.Drives) > 0 {
		guestCfg.Drives = make([]string, len(cfg.Drives))
		for i, drive := range cfg.Drives {
			guestCfg.Drives[i] = drive.PathOnHost
		}
	}

	params := &jailer.PrepareGuestFilesParams{
		Config:       guestCfg,
		AllocDir:     allocDir,
		AllowedPaths: d.config.ImagePaths,
		ChrootPath:   jailerRootDir,
	}

	paths, err := jailer.PrepareGuestFiles(params)
	if err != nil {
		return err
	}

	cfg.BootSource.KernelImagePath = paths.Kernel
	cfg.BootSource.InitrdPath = paths.Initrd
	for i := range cfg.Drives {
		if i < len(paths.Drives) {
			cfg.Drives[i].PathOnHost = paths.Drives[i]
		}
	}

	return nil
}

func (d *FirecrackerDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var handle *drivers.TaskHandle
	var err error

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to decode driver config: %v", err)
	}

	if err := driverConfig.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid task configuration: %v", err)
	}

	if d.config == nil || d.config.Jailer == nil {
		return nil, nil, errors.New("jailer configuration missing")
	}
	jConfig := d.config.Jailer

	jID := jailerID(cfg)

	// Validate socket path length early, before creating any directories.
	if err := jailer.ValidateSocketPath(jConfig.ChrootBase, jID, jConfig.ExecFile); err != nil {
		return nil, nil, err
	}

	// Clean any leftover chroot from a previous run. On a Nomad task
	// restart (StopTask → StartTask, no DestroyTask), the old chroot is
	// still present. The jailer requires a clean directory tree on start.
	jailerPath := jailer.TaskDir(jConfig.ChrootBase, jID, jConfig.ExecFile)
	if err := os.RemoveAll(jailerPath); err != nil {
		return nil, nil, fmt.Errorf("failed to clean existing jailer chroot %s: %v", jailerPath, err)
	}

	paths, err := jailer.BuildPaths(jConfig.ChrootBase, jID, jConfig.ExecFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create jailer paths: %v", err)
	}

	configPath := paths.ConfigPathHost
	configPathChroot := paths.ConfigPathChroot

	if err := d.prepareGuestFiles(&driverConfig, configPath, cfg.AllocDir); err != nil {
		_ = os.RemoveAll(jailerPath)
		return nil, nil, err
	}

	// When Nomad provides network isolation (bridge/group mode) and the user
	// didn't manually configure network interfaces, create a TAP device with
	// TC redirect inside the namespace for seamless bridge networking.
	if cfg.NetworkIsolation != nil && cfg.NetworkIsolation.Path != "" && len(driverConfig.NetworkInterfaces) == 0 {
		tapName, tapErr := network_interface.SetupTapRedirect(cfg.NetworkIsolation.Path)
		if tapErr != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("failed to setup bridge networking: %v", tapErr)
		}
		driverConfig.NetworkInterfaces = network_interface.NetworkInterfaces{
			{
				StaticConfiguration: &network_interface.StaticNetworkConfiguration{
					HostDevName: tapName,
				},
			},
		}
		d.logger.Debug("created tap for bridge networking", "tap", tapName, "netns", cfg.NetworkIsolation.Path)
	}

	vmCfg := &machine.Config{
		BootSource:        driverConfig.BootSource,
		Drives:            driverConfig.Drives,
		NetworkInterfaces: driverConfig.NetworkInterfaces,
	}

	// MMDS requires at least one network interface to route metadata requests.
	if driverConfig.Metadata != "" && len(driverConfig.NetworkInterfaces) == 0 {
		_ = os.RemoveAll(jailerPath)
		return nil, nil, fmt.Errorf("metadata requires networking: configure bridge mode or a network_interface block")
	}

	// When metadata is provided and network interfaces are configured,
	// enable MMDS on the first network interface so the guest can query
	// instance metadata at 169.254.169.254 (Firecracker default).
	if driverConfig.Metadata != "" && len(driverConfig.NetworkInterfaces) > 0 {
		vmCfg.MmdsConfig = &models.MmdsConfig{
			NetworkInterfaces: []string{"eth0"},
		}
	}

	// Check whether a previous snapshot exists for fast restore.
	restoreFromSnapshot := driverConfig.SnapshotOnStop && snapshot.Has(d.config.SnapshotPath, cfg.TaskDir().Dir, cfg.JobName, cfg.Name)

	snapshotPath := d.config.SnapshotPath
	jobID := cfg.JobName
	taskName := cfg.Name

	if restoreFromSnapshot {
		// Link snapshot files into the chroot so Firecracker can load them.
		chrootRoot := filepath.Dir(configPath)
		if err := snapshot.Link(snapshotPath, cfg.TaskDir().Dir, jobID, taskName, chrootRoot); err != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("failed to link snapshot files: %v", err)
		}
		d.logger.Info("restoring from snapshot", "task_id", cfg.ID)
	} else {
		_, err = machine.BuildVMConfig(configPath, vmCfg, cfg.Resources)
		if err != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("failed to build vm configuration: %v", err)
		}
		d.logger.Debug("generated vm configuration", "path", configPath)
	}

	d.logger.Info("starting task", "task_id", cfg.ID, "alloc_id", cfg.AllocID)
	if len(driverConfig.NetworkInterfaces) > 0 {
		d.logger.Debug("network configuration", "network", driverConfig.NetworkInterfaces)
	}
	handle = drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	executorConfig := &executor.ExecutorConfig{
		LogFile:  filepath.Join(cfg.TaskDir().Dir, fmt.Sprintf("%s-executor.out", cfg.Name)),
		LogLevel: "debug",
		Compute:  d.nomadConfig.Topology.Compute(),
	}

	execImpl, pluginClient, err := executor.CreateExecutor(d.logger, d.nomadConfig, executorConfig)
	if err != nil {
		_ = os.RemoveAll(jailerPath)
		return nil, nil, fmt.Errorf("failed to create executor: %v", err)
	}

	socketPath := jailer.SocketPath(jailerPath)

	defer func() {
		if err != nil {
			if execImpl != nil {
				execImpl.Shutdown("", 0)
			}
			if pluginClient != nil {
				pluginClient.Kill()
			}
			_ = os.RemoveAll(jailerPath)
		}
	}()

	params := &jailer.BuildParams{
		ID: jID,
	}

	if cfg.User != "" {
		uid, gid, resolveErr := jailer.ResolveUserIDs(cfg.User)
		if resolveErr != nil {
			err = fmt.Errorf("failed to resolve task user %q for jailer: %v", cfg.User, resolveErr)
			return nil, nil, err
		}
		params.UID = uid
		params.GID = gid
	}

	if cfg.NetworkIsolation != nil && cfg.NetworkIsolation.Path != "" {
		params.NetNS = cfg.NetworkIsolation.Path
	}

	if cgroupVersion := jailer.DetectCgroupVersion(); cgroupVersion != "" {
		params.CgroupVersion = cgroupVersion
	}

	// Resolve to absolute paths to prevent the executor from resolving
	// a relative name against the task directory (binary hijack).
	jailerBin, err := exec.LookPath(jConfig.Bin())
	if err != nil {
		err = fmt.Errorf("jailer binary %q not found in PATH: %v", jConfig.Bin(), err)
		return nil, nil, err
	}

	fcBin, err := exec.LookPath(jConfig.ExecFile)
	if err != nil {
		err = fmt.Errorf("firecracker binary %q not found in PATH: %v", jConfig.ExecFile, err)
		return nil, nil, err
	}

	// Copy the jailer configuration to avoid mutating shared plugin state.
	localJConfig := *jConfig
	localJConfig.ExecFile = fcBin

	// When restoring from snapshot, start Firecracker in API-only mode
	// (no --config-file) so we can call LoadSnapshot before any other config.
	var fcArgs []string
	if !restoreFromSnapshot {
		fcArgs = []string{"--config-file", configPathChroot}
	}

	jArgs, err := localJConfig.BuildArgs(params, fcArgs...)
	if err != nil {
		err = fmt.Errorf("invalid jailer configuration: %v", err)
		return nil, nil, err
	}
	execCmd := &executor.ExecCommand{
		Cmd:        jailerBin,
		Args:       jArgs,
		Env:        cfg.EnvList(),
		TaskDir:    cfg.TaskDir().Dir,
		StdoutPath: cfg.StdoutPath,
		StderrPath: cfg.StderrPath,
		Resources:  cfg.Resources.Copy(),
	}

	ps, err := execImpl.Launch(execCmd)
	if err != nil {
		err = fmt.Errorf("failed to launch command with executor: %v", err)
		return nil, nil, err
	}

	d.logger.Info("firecracker process launched", "task_id", cfg.ID, "pid", ps.Pid)
	d.logger.Debug("jailer command", "cmd", jailerBin, "args", jArgs, "socket", socketPath)

	// Wait for the Firecracker API socket to become ready. The socket is
	// created by the VMM process before the guest boots, so if it isn't
	// available within the timeout the process failed to start.
	// Match the firecracker-go-sdk default of 3s; the VMM creates the
	// socket in 6-60ms (typically ~12ms) per Firecracker's spec.
	if waitErr := client.WaitForReady(d.ctx, socketPath, 3*time.Second); waitErr != nil {
		err = fmt.Errorf("firecracker socket not ready: %v", waitErr)
		return nil, nil, err
	}

	// When restoring from snapshot, load it via API and resume the VM.
	// On failure, remove the snapshot so the next restart cold boots.
	if restoreFromSnapshot {
		loadCtx, loadCancel := context.WithTimeout(d.ctx, 5*time.Second)
		defer loadCancel()
		c := client.New(socketPath)
		if loadErr := c.LoadSnapshot(loadCtx, snapshot.VMStatePath, snapshot.MemPath); loadErr != nil {
			d.logger.Warn("snapshot restore failed, removing snapshot for cold boot on next restart", "task_id", cfg.ID, "err", loadErr)
			_ = snapshot.RemoveDir(snapshotPath, cfg.TaskDir().Dir, jobID, taskName)
			err = fmt.Errorf("failed to load snapshot: %v", loadErr)
			return nil, nil, err
		}
		d.logger.Info("restored from snapshot", "task_id", cfg.ID)
	}

	// If the user provided MMDS metadata, push it to the VM.
	// MMDS data store is not persisted across snapshots, so this
	// runs for both cold boot and snapshot restore.
	if driverConfig.Metadata != "" {
		var metadata interface{}
		if jsonErr := json.Unmarshal([]byte(driverConfig.Metadata), &metadata); jsonErr != nil {
			err = fmt.Errorf("failed to parse MMDS metadata JSON: %v", jsonErr)
			return nil, nil, err
		}
		mmdsCtx, mmdsCancel := context.WithTimeout(d.ctx, 5*time.Second)
		defer mmdsCancel()
		c := client.New(socketPath)
		if mmdsErr := c.PutMmds(mmdsCtx, metadata); mmdsErr != nil {
			err = fmt.Errorf("failed to set MMDS metadata: %v", mmdsErr)
			return nil, nil, err
		}
		d.logger.Info("MMDS metadata configured", "task_id", cfg.ID)
	}

	h := &taskHandle{
		exec:         execImpl,
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
	d.logger.Info("recovering task", "task_id", handle.Config.ID, "pid", taskState.Pid)

	if d.config == nil || d.config.Jailer == nil {
		return fmt.Errorf("cannot recover task: jailer configuration missing")
	}

	plugRC, err := structs.ReattachConfigToGoPlugin(taskState.ReattachConfig)
	if err != nil {
		return fmt.Errorf("failed to build ReattachConfig from taskConfig state: %v", err)
	}

	execImpl, pluginClient, err := executor.ReattachToExecutor(plugRC, d.logger, d.nomadConfig.Topology.Compute())
	if err != nil {
		return fmt.Errorf("failed to reattach to executor: %v", err)
	}

	socketPath, err := jailer.FindTaskSocketPath(d.config.Jailer.ChrootBase, jailerID(taskState.TaskConfig))
	if err != nil {
		d.logger.Warn("failed to discover firecracker socket path after recovery", "task_id", taskState.TaskConfig.ID, "err", err)
		socketPath = ""
	} else if socketPath != "" {
		if err := client.WaitForReady(d.ctx, socketPath, 5*time.Second); err != nil {
			d.logger.Warn("socket not ready after recovery", "task_id", taskState.TaskConfig.ID, "err", err)
			socketPath = ""
		}
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
		if ps == nil {
			result.ExitCode = -1
			result.OOMKilled = false
		}
	} else {
		result = &drivers.ExitResult{
			ExitCode:  ps.ExitCode,
			Signal:    ps.Signal,
			OOMKilled: ps.OOMKilled,
		}
	}

	select {
	case <-ctx.Done():
	case <-d.ctx.Done():
	case ch <- result:
	}
}

func (d *FirecrackerDriverPlugin) StopTask(taskID string, timeout time.Duration, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	// Track elapsed time so exec.Shutdown gets only the remaining budget,
	// matching Docker's single-deadline approach.
	gracefulStart := time.Now()

	if handle.socketPath == "" {
		d.logger.Debug("socket path not available, forcing shutdown", "task_id", taskID)
	} else if d.snapshotEnabled(handle) {
		// Pause the VM and create a snapshot for fast restore on next start.
		// If any step fails the snapshot is discarded; the process is killed
		// below regardless.
		d.snapshotOnStop(handle, timeout)
	} else {
		// Graceful shutdown via Ctrl+Alt+Del, then poll until exit or timeout.
		apiCtx, apiCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer apiCancel()
		c := client.New(handle.socketPath)
		if err := c.SendCtrlAltDel(apiCtx); err != nil {
			d.logger.Debug("graceful shutdown via ctrl+alt+del failed", "task_id", taskID, "err", err)
		} else {
			d.logger.Debug("graceful shutdown initiated via ctrl+alt+del", "task_id", taskID)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
		out:
			for {
				select {
				case <-ctx.Done():
					break out
				case <-ticker.C:
					if !handle.IsRunning() {
						break out
					}
				}
			}
		}
	}

	remaining := timeout - time.Since(gracefulStart)
	if remaining <= 0 {
		remaining = time.Second
	}

	if err := handle.exec.Shutdown(signal, remaining); err != nil {
		if handle.pluginClient.Exited() {
			return nil
		}
		return fmt.Errorf("executor Shutdown failed: %v", err)
	}

	return nil
}

// snapshotEnabled reports whether the snapshot block is set for the task.
func (d *FirecrackerDriverPlugin) snapshotEnabled(handle *taskHandle) bool {
	if handle.taskConfig == nil {
		return false
	}
	var dc TaskConfig
	if err := handle.taskConfig.DecodeDriverConfig(&dc); err != nil {
		return false
	}
	return dc.SnapshotOnStop
}

// snapshotOnStop pauses the VM and creates a snapshot for fast restore on
// next start. On failure, logs a warning and removes any partial snapshot;
// the caller kills the process regardless.
func (d *FirecrackerDriverPlugin) snapshotOnStop(handle *taskHandle, timeout time.Duration) {
	taskID := handle.taskConfig.ID

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := client.New(handle.socketPath)
	if err := c.PauseVM(ctx); err != nil {
		d.logger.Warn("snapshot: failed to pause VM", "task_id", taskID, "err", err)
		return
	}

	if err := c.CreateSnapshot(ctx, snapshot.VMStatePath, snapshot.MemPath); err != nil {
		d.logger.Warn("snapshot: failed to create snapshot", "task_id", taskID, "err", err)
		return
	}

	// Move snapshot files out of the chroot so they survive DestroyTask.
	// Derive the chroot root from the socket path:
	//   <chrootBase>/<exec>/<id>/root/run/firecracker.socket → .../root
	chrootRoot := filepath.Dir(filepath.Dir(handle.socketPath))
	snapshotPath := d.config.SnapshotPath
	taskDir := handle.taskConfig.TaskDir().Dir
	jobID := handle.taskConfig.JobName
	taskName := handle.taskConfig.Name
	if err := snapshot.Save(chrootRoot, snapshotPath, taskDir, jobID, taskName); err != nil {
		d.logger.Warn("snapshot: failed to save snapshot files", "task_id", taskID, "err", err)
		_ = snapshot.RemoveDir(snapshotPath, taskDir, jobID, taskName)
		return
	}

	d.logger.Info("snapshot saved for fast restart", "task_id", taskID)
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

	var dirs []string
	if handle.socketPath != "" {
		dirs = []string{jailer.TaskDirFromSocketPath(handle.socketPath)}
	} else {
		var findErr error
		dirs, findErr = jailer.FindAllTaskDirs(d.config.Jailer.ChrootBase, jailerID(handle.taskConfig))
		if findErr != nil {
			handle.logger.Warn("failed to discover jailer directory for cleanup", "task_id", handle.taskConfig.ID, "err", findErr)
		}
	}
	for _, dir := range dirs {
		if err := os.RemoveAll(dir); err != nil {
			handle.logger.Warn("failed to clean up jailer directory", "path", dir, "err", err)
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

func (d *FirecrackerDriverPlugin) SignalTask(taskID string, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return handle.forwardSignal(ctx, signal)
}

func (d *FirecrackerDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	return nil, errors.New("exec is not supported for Firecracker VMs; configure your guest OS to handle command execution externally")
}
