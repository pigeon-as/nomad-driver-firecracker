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
	"strings"
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

// isAllowedImagePath checks if a path is under the allocation directory or
// within the configured allowlist of image paths. This prevents tenants from
// specifying arbitrary host paths.
// Note: Symlink resolution is performed separately just before hard linking
// to prevent TOCTOU (Time-of-check-time-of-use) vulnerabilities.
func isAllowedImagePath(allowedPaths []string, allocDir, imagePath string) bool {
	if !filepath.IsAbs(imagePath) {
		imagePath = filepath.Join(allocDir, imagePath)
	}

	isParent := func(parent, path string) bool {
		rel, err := filepath.Rel(parent, path)
		return err == nil && !strings.HasPrefix(rel, "..")
	}

	// Check if path is under alloc dir
	if isParent(allocDir, imagePath) {
		return true
	}

	// Check allowed paths
	for _, ap := range allowedPaths {
		if isParent(ap, imagePath) {
			return true
		}
	}

	return false
}

// prepareGuestFiles links guest files (kernel, initrd, drives) into the jailer chroot
// and updates the task config with relative paths for use in the VM config.
// This follows the official Firecracker pattern: files must be hard-linked into the
// chroot because the jailed Firecracker process cannot access host paths.
func (d *FirecrackerDriverPlugin) prepareGuestFiles(cfg *TaskConfig, configPath, allocDir string) error {
	jailorRootDir := filepath.Dir(configPath) // Same as jailer root

	// Validate all image paths
	if cfg.BootSource != nil {
		if cfg.BootSource.KernelImagePath != "" {
			if !isAllowedImagePath(d.config.ImagePaths, allocDir, cfg.BootSource.KernelImagePath) {
				return fmt.Errorf("kernel_image_path %q is not in allowed paths", cfg.BootSource.KernelImagePath)
			}
			// Convert relative paths to absolute for hard link creation
			if !filepath.IsAbs(cfg.BootSource.KernelImagePath) {
				cfg.BootSource.KernelImagePath = filepath.Join(allocDir, cfg.BootSource.KernelImagePath)
			}
		}
		if cfg.BootSource.InitrdPath != "" {
			if !isAllowedImagePath(d.config.ImagePaths, allocDir, cfg.BootSource.InitrdPath) {
				return fmt.Errorf("initrd_path %q is not in allowed paths", cfg.BootSource.InitrdPath)
			}
			// Convert relative paths to absolute for hard link creation
			if !filepath.IsAbs(cfg.BootSource.InitrdPath) {
				cfg.BootSource.InitrdPath = filepath.Join(allocDir, cfg.BootSource.InitrdPath)
			}
		}
	}

	for i, drive := range cfg.Drives {
		if drive.PathOnHost != "" {
			if !isAllowedImagePath(d.config.ImagePaths, allocDir, drive.PathOnHost) {
				return fmt.Errorf("drive[%d].path_on_host %q is not in allowed paths", i, drive.PathOnHost)
			}
			// Convert relative paths to absolute for hard link creation
			if !filepath.IsAbs(drive.PathOnHost) {
				cfg.Drives[i].PathOnHost = filepath.Join(allocDir, drive.PathOnHost)
			}
		}
	}

	// Build request with guest files to link
	req := &jailer.LinkGuestFilesRequest{}

	// Resolve symlinks immediately before linking to prevent TOCTOU attacks
	if cfg.BootSource != nil {
		if cfg.BootSource.KernelImagePath != "" {
			resolvedKernel, err := filepath.EvalSymlinks(cfg.BootSource.KernelImagePath)
			if err != nil {
				return fmt.Errorf("failed to resolve kernel symlink: %w", err)
			}
			req.KernelImagePath = resolvedKernel
		}
		if cfg.BootSource.InitrdPath != "" {
			resolvedInitrd, err := filepath.EvalSymlinks(cfg.BootSource.InitrdPath)
			if err != nil {
				return fmt.Errorf("failed to resolve initrd symlink: %w", err)
			}
			req.InitrdPath = resolvedInitrd
		}
	}

	if len(cfg.Drives) > 0 {
		req.DrivePaths = make([]string, len(cfg.Drives))
		for i, drive := range cfg.Drives {
			if drive.PathOnHost != "" {
				resolvedDrive, err := filepath.EvalSymlinks(drive.PathOnHost)
				if err != nil {
					return fmt.Errorf("failed to resolve drive[%d] symlink: %w", i, err)
				}
				req.DrivePaths[i] = resolvedDrive
			}
		}
	}

	// Link files into chroot and get relative paths
	linkedPaths, err := jailer.LinkGuestFilesForTask(jailorRootDir, req)
	if err != nil {
		return fmt.Errorf("failed to link guest files into jailer chroot: %w", err)
	}

	// Update config with relative paths (as seen from inside chroot)
	if cfg.BootSource != nil && cfg.BootSource.KernelImagePath != "" {
		if relativeName, ok := linkedPaths[cfg.BootSource.KernelImagePath]; ok {
			cfg.BootSource.KernelImagePath = relativeName
		}
	}

	if cfg.BootSource != nil && cfg.BootSource.InitrdPath != "" {
		if relativeName, ok := linkedPaths[cfg.BootSource.InitrdPath]; ok {
			cfg.BootSource.InitrdPath = relativeName
		}
	}

	for i, drive := range cfg.Drives {
		if drive.PathOnHost != "" {
			if relativeName, ok := linkedPaths[drive.PathOnHost]; ok {
				cfg.Drives[i].PathOnHost = relativeName
			}
		}
	}

	d.logger.Debug("guest files linked into jailer chroot", "file_count", len(linkedPaths))
	return nil
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

	// Link guest files (kernel, initrd, drives) into jailer chroot and update config with relative paths
	if err := d.prepareGuestFiles(&driverConfig, configPath, cfg.AllocDir); err != nil {
		// Clean up entire config directory to remove any hard links that may have been created
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
		// Clean up entire config directory to remove vmconfig.json and any hard links
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
		return nil, nil, fmt.Errorf("failed to create executor: %v", err)
	}

	// Derive socket path early for potential cleanup and later use
	jailorPath := filepath.Join(cfg.TaskDir().Dir, "jailer", cfg.ID)
	socketPath := filepath.Join(jailorPath, "root", "run", "firecracker.socket")

	// Guarantee cleanup of executor resources and jailer directory on any error
	defer func() {
		if err != nil {
			pluginClient.Kill()
			if shutdownErr := exec.Shutdown("", 0); shutdownErr != nil {
				d.logger.Error("failed to shutdown executor on error", "error", shutdownErr)
			}
			// Clean up jailer directory on startup failure
			if rmErr := os.RemoveAll(jailorPath); rmErr != nil {
				d.logger.Warn("failed to clean up jailer directory on error", "path", jailorPath, "err", rmErr)
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

	jArgs, err := jConfig.BuildArgs(cfg.TaskDir().Dir, params, "--config-file", configPathChroot)
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

	// Verify socket is accessible before returning handle. Socket is required for VM management.
	if err := d.waitForSocket(socketPath, 5*time.Second); err != nil {
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

	// Create client once outside the loop
	c := client.New(socketPath)

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
			// SDK's GetMachineConfiguration uses the client's global firecrackerRequestTimeout (default 500ms).
			_, err := c.GetMachineConfiguration()

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

	socketPath := filepath.Join(taskState.TaskConfig.TaskDir().Dir, "jailer", taskState.TaskConfig.ID, "root", "run", "firecracker.socket")
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
	// and wait for VM to exit before forcing shutdown
	if signal == "SIGTERM" || signal == "SIGINT" {
		err := handle.forwardSignal(context.Background(), signal, 5*time.Second)
		if err == nil {
			// Graceful shutdown initiated, wait for VM to exit
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					// Timeout expired, fall through to force shutdown
					d.logger.Warn("graceful shutdown timed out, forcing shutdown", "task_id", taskID, "timeout", timeout)
					goto forceShutdown
				case <-ticker.C:
					if !handle.IsRunning() {
						// VM exited gracefully
						return nil
					}
				}
			}
		}
		d.logger.Debug("graceful shutdown failed, falling back to forced shutdown", "task_id", taskID, "err", err)
	}

forceShutdown:
	// Force shutdown via executor
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
