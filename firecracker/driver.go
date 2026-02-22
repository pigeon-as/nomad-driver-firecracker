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
	"regexp"
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
)

var versionRegex = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

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
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = "firecracker binary not configured"
		return fp
	}

	bin := d.config.Jailer.ExecFile
	binPath, err := exec.LookPath(bin)
	if err != nil {
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = fmt.Sprintf("firecracker binary %s not found: %v", bin, err)
		return fp
	}

	version := queryVersion(binPath)
	if version != "" {
		fp.Attributes["driver.firecracker.version"] = structs.NewStringAttribute(version)
	}

	return fp
}

// queryVersion extracts the version string from a binary's --version output.
func queryVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return versionRegex.FindString(string(out))
}

// prepareGuestFiles orchestrates guest file preparation by delegating to the jailer package.
// The jailer package handles all path validation, symlink resolution, and file linking.
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

	d.logger.Debug("guest files linked into jailer chroot")
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

	paths, err := jailer.BuildPaths(cfg.TaskDir().Dir, cfg.ID, jConfig.ExecFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create jailer paths: %v", err)
	}

	configPath := paths.ConfigPathHost
	configPathChroot := paths.ConfigPathChroot

	if err := d.prepareGuestFiles(&driverConfig, configPath, cfg.AllocDir); err != nil {
		_ = os.RemoveAll(filepath.Dir(configPath))
		return nil, nil, err
	}

	vmCfg := &machine.Config{
		BootSource:        driverConfig.BootSource,
		Drives:            driverConfig.Drives,
		NetworkInterfaces: driverConfig.NetworkInterfaces,
	}
	_, err = machine.BuildVMConfig(configPath, vmCfg, cfg.Resources)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(configPath))
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
		_ = os.RemoveAll(filepath.Dir(configPath))
		return nil, nil, fmt.Errorf("failed to create executor: %v", err)
	}

	// Derive socket path early for potential cleanup and later use.
	// Follows Firecracker jailer layout: <chroot_base>/<exec_file_name>/<id>/root
	execFileName := filepath.Base(jConfig.ExecFile)
	jailerPath := filepath.Join(cfg.TaskDir().Dir, "jailer", execFileName, cfg.ID)
	socketPath := filepath.Join(jailerPath, "root", "run", "firecracker.socket")

	// Guarantee cleanup of executor resources and jailer directory on any error.
	// Shutdown executor before killing plugin client to allow graceful RPC cleanup.
	defer func() {
		if err != nil {
			if exec != nil {
				exec.Shutdown("", 1*time.Second)
			}
			if pluginClient != nil {
				pluginClient.Kill()
			}
			// Clean up jailer directory and config on startup failure
			_ = os.RemoveAll(jailerPath)
		}
	}()

	params := &jailer.BuildParams{
		ID: cfg.ID,
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

	// Detect and configure host's cgroup version if available.
	if cgroupVersion := detectCgroupVersion(); cgroupVersion != "" {
		params.CgroupVersion = cgroupVersion
	}

	jArgs, err := jConfig.BuildArgs(cfg.TaskDir().Dir, params, "--config-file", configPathChroot)
	if err != nil {
		err = fmt.Errorf("invalid jailer configuration: %v", err)
		return nil, nil, err
	}
	execCmd := &executor.ExecCommand{
		Cmd:        jConfig.Bin(),
		Args:       jArgs,
		Env:        cfg.EnvList(),
		TaskDir:    cfg.TaskDir().Dir,
		StdoutPath: cfg.StdoutPath,
		StderrPath: cfg.StderrPath,
		Resources:  cfg.Resources,
	}

	ps, err := exec.Launch(execCmd)
	if err != nil {
		err = fmt.Errorf("failed to launch command with executor: %v", err)
		return nil, nil, err
	}

	d.logger.Info("firecracker process launched", "task_id", cfg.ID, "pid", ps.Pid)

	// Verify socket is accessible before returning handle. Socket is required for VM management.
	// waitForSocket polls until socket is ready or timeout expires.
	if err = d.waitForSocket(socketPath, 5*time.Second); err != nil {
		err = fmt.Errorf("firecracker socket not ready after startup: %v", err)
		return nil, nil, err
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

	// Build network information from configured interfaces
	var driverNetwork *drivers.DriverNetwork
	if len(driverConfig.NetworkInterfaces) > 0 {
		driverNetwork = &drivers.DriverNetwork{
			PortMap: map[string]int{},
		}
	}

	return handle, driverNetwork, nil
}

func (d *FirecrackerDriverPlugin) waitForSocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	c := client.New(socketPath)

	for {
		select {
		case <-ticker.C:
			if _, err := c.GetMachineConfiguration(); err == nil {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("firecracker socket not ready after %v", timeout)
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

	plugRC, err := structs.ReattachConfigToGoPlugin(taskState.ReattachConfig)
	if err != nil {
		return fmt.Errorf("failed to build ReattachConfig from taskConfig state: %v", err)
	}

	execImpl, pluginClient, err := executor.ReattachToExecutor(plugRC, d.logger, d.nomadConfig.Topology.Compute())
	if err != nil {
		return fmt.Errorf("failed to reattach to executor: %v", err)
	}

	// Reconstruct socket path using jailer layout: <chroot_base>/<exec_file_name>/<id>/root
	execFileName := filepath.Base(d.config.Jailer.ExecFile)
	socketPath := filepath.Join(taskState.TaskConfig.TaskDir().Dir, "jailer", execFileName, taskState.TaskConfig.ID, "root", "run", "firecracker.socket")
	if err := d.waitForSocket(socketPath, 5*time.Second); err != nil {
		d.logger.Warn("socket not ready after recovery", "task_id", taskState.TaskConfig.ID, "err", err)
		socketPath = "" // Clear socket path if not ready; signals will fail gracefully
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
		// If process state is nil, we've probably been killed, so return a reasonable exit code
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

	// Attempt graceful VM shutdown via Ctrl+Alt+Del if socket is available.
	// This mirrors the QEMU driver's graceful shutdown via monitor socket:
	// send the shutdown request, then poll until the process exits or timeout.
	if handle.socketPath != "" {
		// Use a short timeout for the API call itself to avoid hanging
		// indefinitely if the socket is broken.
		apiCtx, apiCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer apiCancel()
		c := client.New(handle.socketPath)
		if err := c.SendCtrlAltDel(apiCtx); err != nil {
			d.logger.Debug("graceful shutdown via ctrl+alt+del failed", "task_id", taskID, "err", err)
		} else {
			d.logger.Debug("graceful shutdown initiated via ctrl+alt+del", "task_id", taskID)
			// Wait for the VM to shut down gracefully before falling back
			// to exec.Shutdown, matching the QEMU driver pattern.
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
		out:
			for {
				select {
				case <-ctx.Done():
					d.logger.Debug("graceful shutdown timed out, forcing shutdown", "task_id", taskID, "timeout", timeout)
					break out
				case <-ticker.C:
					if !handle.IsRunning() {
						break out
					}
				}
			}
		}
	} else {
		d.logger.Debug("socket path not available, forcing shutdown", "task_id", taskID)
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

	// Clean up jailer directory structure using correct jailer layout
	if handle.taskConfig != nil && handle.taskConfig.TaskDir() != nil {
		execFileName := filepath.Base(d.config.Jailer.ExecFile)
		jailerPath := filepath.Join(handle.taskConfig.TaskDir().Dir, "jailer", execFileName, handle.taskConfig.ID)
		if err := os.RemoveAll(jailerPath); err != nil {
			handle.logger.Warn("failed to clean up jailer directory", "path", jailerPath, "err", err)
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

// detectCgroupVersion returns the host's cgroup version ("1" or "2"), or empty string if unknown.
// Values match Firecracker jailer --cgroup-version argument (default "1").
func detectCgroupVersion() string {
	// Check for cgroups v2 unified hierarchy
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return "2"
	}
	// Check for cgroups v1 with cpu subsystem
	if _, err := os.Stat("/sys/fs/cgroup/cpu"); err == nil {
		return "1"
	}
	return ""
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

	// Use a bounded timeout for the API call to avoid blocking indefinitely
	// if the Firecracker socket is stalled, matching StopTask's approach.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return handle.forwardSignal(ctx, signal)
}

func (d *FirecrackerDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	return nil, errors.New("exec is not supported for Firecracker VMs; configure your guest OS to handle command execution externally")
}
