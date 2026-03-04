// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package firecracker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/drivers/shared/eventer"
	"github.com/hashicorp/nomad/drivers/shared/executor"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"github.com/hashicorp/nomad/plugins/shared/structs"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/guestapi"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/jailer"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/machine"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/network"
	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/snapshot"
)

const (
	pluginName = "firecracker"

	pluginVersion = "v0.0.1"

	fingerprintPeriod = 30 * time.Second

	taskHandleVersion = 1
)

// jailerID returns a unique, filesystem-safe identifier for the jailer.
// The jailer requires IDs matching ^[a-zA-Z0-9-]{1,64}$.
// AllocID (36 chars) + "-" + 8-char hex hash of task name = 45 chars.
func jailerID(cfg *drivers.TaskConfig) string {
	h := sha256.Sum256([]byte(cfg.Name))
	return cfg.AllocID + "-" + hex.EncodeToString(h[:4])
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

func (d *FirecrackerDriverPlugin) prepareGuestFiles(cfg *TaskConfig, chrootRoot, allocDir string) error {
	if d.config == nil {
		return fmt.Errorf("driver configuration not initialized")
	}

	jailerRootDir := chrootRoot

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

	chrootRoot, err := jailer.BuildChrootDir(jConfig.ChrootBase, jID, jConfig.ExecFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create jailer chroot: %v", err)
	}

	if err := d.prepareGuestFiles(&driverConfig, chrootRoot, cfg.AllocDir); err != nil {
		_ = os.RemoveAll(jailerPath)
		return nil, nil, err
	}

	// When Nomad provides network isolation (bridge/group mode) and the user
	// didn't manually configure network interfaces, create a TAP device with
	// TC redirect inside the namespace for seamless bridge networking.
	var guestNet *network.GuestNetworkConfig
	if cfg.NetworkIsolation != nil && cfg.NetworkIsolation.Path != "" && len(driverConfig.NetworkInterfaces) == 0 {
		nifs, netCfg, tapErr := network.AutoSetup(cfg.NetworkIsolation.Path)
		if tapErr != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("failed to setup bridge networking: %v", tapErr)
		}
		driverConfig.NetworkInterfaces = nifs
		guestNet = netCfg
		d.logger.Debug("created tap for bridge networking", "tap", nifs[0].StaticConfiguration.HostDevName, "netns", cfg.NetworkIsolation.Path)
		if guestNet != nil {
			d.logger.Debug("read guest network config from veth", "ip", guestNet.IP, "mask", guestNet.Mask, "gw", guestNet.Gateway)
		}
	}

	// When Nomad provides host volume mounts and the host path is a block
	// device, auto-attach it as a Firecracker drive. This follows the same
	// opt-in pattern as bridge networking: only activates when Nomad provides
	// volumes via MountConfig, and the device is a block special file.
	// The resulting MMDS Mounts config tells pigeon-init where to mount
	// each device inside the guest.
	var guestMounts []machine.GuestMount
	if len(cfg.Mounts) > 0 {
		for _, m := range cfg.Mounts {
			info, statErr := os.Stat(m.HostPath)
			if statErr != nil {
				_ = os.RemoveAll(jailerPath)
				return nil, nil, fmt.Errorf("volume mount %q: %v", m.HostPath, statErr)
			}
			if info.Mode()&os.ModeDevice == 0 {
				_ = os.RemoveAll(jailerPath)
				return nil, nil, fmt.Errorf("volume mount %q is not a block device; the firecracker driver only supports block device volume mounts", m.HostPath)
			}

			// Calculate the virtio-blk device letter based on drive index.
			// User-specified drives come first, volume drives are appended.
			driveIdx := len(driverConfig.Drives)
			if driveIdx > 25 {
				_ = os.RemoveAll(jailerPath)
				return nil, nil, fmt.Errorf("too many drives (%d); maximum 26 virtio-blk devices supported", driveIdx+1)
			}
			devLetter := string(rune('a' + driveIdx))
			guestDev := "/dev/vd" + devLetter

			driverConfig.Drives = append(driverConfig.Drives, machine.Drive{
				PathOnHost:   m.HostPath,
				IsRootDevice: false,
				IsReadOnly:   m.Readonly,
			})

			guestMounts = append(guestMounts, machine.GuestMount{
				DevicePath: guestDev,
				MountPath:  m.TaskPath,
			})
		}

		// Link volume block devices into the chroot. prepareGuestFiles
		// only handled user-configured drives (regular files); volume
		// drives are block devices and need mknod instead of hard links.
		volumeStart := len(driverConfig.Drives) - len(guestMounts)
		volumePaths := make([]string, len(guestMounts))
		for i := range guestMounts {
			volumePaths[i] = driverConfig.Drives[volumeStart+i].PathOnHost
		}
		resolved, err := jailer.LinkDeviceNodes(chrootRoot, volumePaths)
		if err != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("failed to link volume drives into chroot: %v", err)
		}
		for i := range guestMounts {
			driverConfig.Drives[volumeStart+i].PathOnHost = filepath.Base(resolved[i])
		}
	}

	vmCfg := &machine.Config{
		BootSource:        driverConfig.BootSource,
		Drives:            driverConfig.Drives,
		NetworkInterfaces: driverConfig.NetworkInterfaces,
		Balloon:           driverConfig.Balloon,
		Vsock:             driverConfig.Vsock,
		LogLevel:          driverConfig.LogLevel,
		Mmds:              driverConfig.Mmds,
	}

	// Check whether a previous snapshot exists for fast restore.
	snapLoc := snapshot.Loc{TaskDir: cfg.TaskDir().Dir}
	restoreFromSnapshot := driverConfig.SnapshotOnStop && snapLoc.Has()

	// Validate the VM configuration eagerly, before launching the process.
	var vmSDK *models.FullVMConfiguration
	if !restoreFromSnapshot {
		vmSDK, err = machine.ToSDK(vmCfg, cfg.Resources)
		if err != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("invalid vm configuration: %v", err)
		}
	}

	if restoreFromSnapshot {
		// Link snapshot files into the chroot so Firecracker can load them.
		if err := snapLoc.Link(chrootRoot); err != nil {
			_ = os.RemoveAll(jailerPath)
			return nil, nil, fmt.Errorf("failed to link snapshot files: %v", err)
		}
		d.logger.Info("restoring from snapshot", "task_id", cfg.ID)
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

	// Resolve optional task user for the jailer. When omitted, the SDK
	// defaults to UID/GID 0 (root), matching Firecracker's production
	// documentation. Operators can set the Nomad task 'user' stanza to
	// run the jailer as a non-root user.
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

	// Always start Firecracker in API-only mode (no --config-file),
	// matching the firecracker-go-sdk pattern. Configuration is applied
	// via sequential API calls after the socket is ready.
	jArgs, err := localJConfig.BuildArgs(params)
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
	// Matches the firecracker-go-sdk default of 3s with 10ms polling.
	if waitErr := machine.WaitForReady(d.ctx, socketPath, 3*time.Second); waitErr != nil {
		err = fmt.Errorf("firecracker socket not ready: %v", waitErr)
		return nil, nil, err
	}

	// Create the Firecracker log file inside the chroot before calling
	// PUT /logger. The file path on the host maps into the chroot via
	// pivot_root, so Firecracker sees it at its working directory root.
	// Matches the firecracker-go-sdk CreateLogFilesHandler pattern.
	logFile := filepath.Join(chrootRoot, machine.LogFile)
	f, createErr := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if createErr != nil {
		err = fmt.Errorf("failed to create firecracker log file: %v", createErr)
		return nil, nil, err
	}
	if closeErr := f.Close(); closeErr != nil {
		err = fmt.Errorf("failed to close firecracker log file: %v", closeErr)
		return nil, nil, err
	}

	// When the jailer runs as a non-root user, adjust ownership so
	// Firecracker can write to the log file after pivot_root.
	if params.UID != nil && params.GID != nil {
		if chownErr := os.Chown(logFile, *params.UID, *params.GID); chownErr != nil {
			err = fmt.Errorf("failed to chown firecracker log file: %v", chownErr)
			return nil, nil, err
		}
	}

	// Configure the VM via the Firecracker API.
	c := machine.NewClient(socketPath)
	configCtx, configCancel := context.WithTimeout(d.ctx, 10*time.Second)
	defer configCancel()

	// Configure Firecracker logging before VM configuration or snapshot
	// restore. The SDK's handler chain configures logging for both paths.
	// PutLogger redirects structured daemon JSON logs to a file inside
	// the chroot, leaving stdout for guest console output only.
	{
		logPath := machine.LogFile
		logLevel := vmCfg.LogLevel
		if logLevel == "" {
			logLevel = machine.DefaultLogLevel
		}
		showLevel := true
		showLogOrigin := false
		logger := &models.Logger{
			LogPath:       &logPath,
			Level:         &logLevel,
			ShowLevel:     &showLevel,
			ShowLogOrigin: &showLogOrigin,
		}
		if logErr := c.PutLogger(configCtx, logger); logErr != nil {
			err = fmt.Errorf("PUT /logger: %v", logErr)
			return nil, nil, err
		}
		d.logger.Debug("firecracker logger configured", "task_id", cfg.ID, "level", logLevel)
	}

	if restoreFromSnapshot {
		// Load a previously saved snapshot and resume the VM.
		// On failure, remove the snapshot so the next restart cold boots.
		if loadErr := c.LoadSnapshot(configCtx, snapshot.VMStatePath, snapshot.MemPath); loadErr != nil {
			d.logger.Warn("snapshot restore failed, removing snapshot for cold boot on next restart", "task_id", cfg.ID, "err", loadErr)
			_ = snapLoc.RemoveDir()
			err = fmt.Errorf("failed to load snapshot: %v", loadErr)
			return nil, nil, err
		}
		d.logger.Info("restored from snapshot", "task_id", cfg.ID)
	} else {
		// Cold boot: configure each resource via sequential API calls,
		// following the firecracker-go-sdk handler chain order.
		if configErr := c.ConfigureVM(configCtx, vmSDK); configErr != nil {
			err = fmt.Errorf("failed to configure VM via API: %v", configErr)
			return nil, nil, err
		}
		d.logger.Info("VM configured and started via API", "task_id", cfg.ID)
	}

	// Push MMDS metadata to the VM. Content construction lives in the
	// machine package (domain code per AGENTS.md). MMDS routing is
	// already enabled by ToSDK whenever networking exists.
	// MMDS data store is not persisted across snapshots, so this
	// runs for both cold boot and snapshot restore.
	if mmdsContent := machine.BuildMmdsContent(driverConfig.Mmds.GetMetadata(), guestNet, guestMounts); mmdsContent != nil {
		if mmdsErr := c.PutMmds(configCtx, mmdsContent); mmdsErr != nil {
			err = fmt.Errorf("failed to set MMDS metadata: %v", mmdsErr)
			return nil, nil, err
		}
		d.logger.Info("MMDS metadata configured", "task_id", cfg.ID)
	}

	var gc *guestapi.Client
	if driverConfig.GuestAPI != nil && socketPath != "" {
		if uds := guestapi.UDSPath(socketPath); uds != "" {
			gc = guestapi.New(uds, driverConfig.GuestAPI.Port)
		}
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
		guestClient:  gc,
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
		if err := machine.WaitForReady(d.ctx, socketPath, 5*time.Second); err != nil {
			d.logger.Warn("socket not ready after recovery", "task_id", taskState.TaskConfig.ID, "err", err)
			socketPath = ""
		}
	}

	var gc *guestapi.Client
	if socketPath != "" {
		var dc TaskConfig
		if err := taskState.TaskConfig.DecodeDriverConfig(&dc); err == nil && dc.GuestAPI != nil {
			if uds := guestapi.UDSPath(socketPath); uds != "" {
				gc = guestapi.New(uds, dc.GuestAPI.Port)
			}
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
		guestClient:  gc,
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

	// Treat timeout as a single overall deadline, like Docker's Kill.
	// The graceful phase (Ctrl+Alt+Del or snapshot) consumes part of
	// the budget; exec.Shutdown gets whatever remains. When the budget
	// is exhausted, remaining == 0 triggers an immediate proc.Kill().
	deadline := time.Now().Add(timeout)

	if handle.socketPath == "" {
		d.logger.Debug("socket path not available, forcing shutdown", "task_id", taskID)
	} else if d.snapshotEnabled(handle) {
		// Pause the VM and create a snapshot for fast restore on next start.
		// If any step fails the snapshot is discarded; the process is killed
		// below regardless.
		d.snapshotOnStop(handle, time.Until(deadline))
	} else {
		d.gracefulShutdown(handle, taskID, deadline)
	}

	// Give exec.Shutdown only the remaining budget. When remaining is
	// zero the executor sends an immediate SIGKILL (no grace period).
	remaining := time.Until(deadline)
	if remaining < 0 {
		remaining = 0
	}

	if err := handle.exec.Shutdown(signal, remaining); err != nil {
		if handle.pluginClient.Exited() {
			return nil
		}
		return fmt.Errorf("executor Shutdown failed: %v", err)
	}

	return nil
}

// gracefulShutdown attempts to stop the guest workload gracefully within
// the given deadline. Preferred path: vsock SIGTERM (arch-independent).
// Fallback: Ctrl+Alt+Del via Firecracker API (x86_64 only, requires kernel
// keyboard support). Polls until the process exits or the deadline expires.
func (d *FirecrackerDriverPlugin) gracefulShutdown(handle *taskHandle, taskID string, deadline time.Time) {
	apiCtx, apiCancel := context.WithDeadline(context.Background(), deadline)
	defer apiCancel()

	initiated := false

	// Tier 1: vsock SIGTERM — works on all architectures.
	if gc := handle.guestClient; gc != nil {
		if err := gc.Signal(apiCtx, int(syscall.SIGTERM)); err != nil {
			d.logger.Debug("vsock SIGTERM failed", "task_id", taskID, "err", err)
		} else {
			d.logger.Debug("graceful shutdown initiated via vsock SIGTERM", "task_id", taskID)
			initiated = true
		}
	}

	// Tier 2: Ctrl+Alt+Del (x86_64 only, requires kernel support).
	if !initiated {
		c := machine.NewClient(handle.socketPath)
		if err := c.SendCtrlAltDel(apiCtx); err != nil {
			d.logger.Debug("ctrl+alt+del failed", "task_id", taskID, "err", err)
			return // No graceful method worked; fall through to exec.Shutdown.
		}
		d.logger.Debug("graceful shutdown initiated via ctrl+alt+del", "task_id", taskID)
	}

	// Poll until the process exits or the deadline expires.
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !handle.IsRunning() {
				return
			}
		}
	}
}

// snapshotEnabled reports whether snapshot-on-stop is enabled for the task.
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

	c := machine.NewClient(handle.socketPath)
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
	snapLoc := snapshot.Loc{TaskDir: handle.taskConfig.TaskDir().Dir}
	if err := snapLoc.Save(chrootRoot); err != nil {
		d.logger.Warn("snapshot: failed to save snapshot files", "task_id", taskID, "err", err)
		_ = snapLoc.RemoveDir()
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
	return nil, errors.New("exec is not supported by the firecracker driver; use SSH or a vsock-based guest agent for in-guest command execution")
}
