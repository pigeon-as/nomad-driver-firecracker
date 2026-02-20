// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package firecracker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/consul-template/signals"
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
	// pluginName is the name of the plugin
	// this is used for logging and (along with the version) for uniquely
	// identifying plugin binaries fingerprinted by the client
	pluginName = "firecracker"

	// pluginVersion allows the client to identify and use newer versions of
	// an installed plugin
	pluginVersion = "v0.0.1"

	// fingerprintPeriod is the interval at which the plugin will send
	// fingerprint responses
	fingerprintPeriod = 30 * time.Second

	// taskHandleVersion is the version of task handle which this plugin sets
	// and understands how to decode
	// this is used to allow modification and migration of the task schema
	// used by the plugin
	taskHandleVersion = 1
)

var (
	// pluginInfo describes the plugin
	pluginInfo = &base.PluginInfoResponse{
		Type:              base.PluginTypeDriver,
		PluginApiVersions: []string{drivers.ApiVersion010},
		PluginVersion:     pluginVersion,
		Name:              pluginName,
	}
)

// TaskState is the runtime state which is encoded in the handle returned to
// Nomad client.
// This information is needed to rebuild the task state and handler during
// recovery.
type TaskState struct {
	ReattachConfig *structs.ReattachConfig
	TaskConfig     *drivers.TaskConfig
	StartedAt      time.Time
	Pid            int
	SocketPath     string // Unix socket path for communicating with Firecracker VM
}

// FirecrackerDriverPlugin implements the Nomad driver interface for running
// Firecracker microVMs.  The struct is exported only via the factory below –
// the original skeleton example name has been updated to reflect the actual
// driver purpose.
type FirecrackerDriverPlugin struct {
	// eventer is used to handle multiplexing of TaskEvents calls such that an
	// event can be broadcast to all callers
	eventer *eventer.Eventer

	// config is the plugin configuration set by the SetConfig RPC
	config *Config

	// nomadConfig is the client config from Nomad
	nomadConfig *base.ClientDriverConfig

	// tasks is the in memory datastore mapping taskIDs to driver handles
	tasks *taskStore

	// ctx is the context for the driver. It is passed to other subsystems to
	// coordinate shutdown
	ctx context.Context

	// signalShutdown is called when the driver is shutting down and cancels
	// the ctx passed to any subsystems
	signalShutdown context.CancelFunc

	// logger will log to the Nomad agent
	logger hclog.Logger
}

// NewPlugin returns a new example driver plugin
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

// PluginInfo returns information describing the plugin.
func (d *FirecrackerDriverPlugin) PluginInfo() (*base.PluginInfoResponse, error) {
	return pluginInfo, nil
}

// ConfigSchema returns the plugin configuration schema.
func (d *FirecrackerDriverPlugin) ConfigSchema() (*hclspec.Spec, error) {
	return configSpec, nil
}

// SetConfig is called by the client to pass the configuration for the plugin.
func (d *FirecrackerDriverPlugin) SetConfig(cfg *base.Config) error {
	var config Config
	if len(cfg.PluginConfig) != 0 {
		if err := base.MsgPackDecode(cfg.PluginConfig, &config); err != nil {
			return err
		}
	}

	// Save the configuration to the plugin
	d.config = &config

	// validate global configuration now that we have decoded it
	if err := d.config.Validate(); err != nil {
		return err
	}

	// TODO: parse and validated any configuration value if necessary.
	//
	// If your driver agent configuration requires any complex validation
	// (some dependency between attributes) or special data parsing (the
	// string "10s" into a time.Interval) you can do it here and update the
	// value in d.config.
	//

	// Save the Nomad agent configuration
	if cfg.AgentConfig != nil {
		d.nomadConfig = cfg.AgentConfig.Driver
	}

	// TODO: initialize any extra requirements if necessary.
	//
	// Here you can use the config values to initialize any resources that are
	// shared by all tasks that use this driver, such as a daemon process.

	return nil
}

// TaskConfigSchema returns the HCL schema for the configuration of a task.
func (d *FirecrackerDriverPlugin) TaskConfigSchema() (*hclspec.Spec, error) {
	return taskConfigSpec, nil
}

// Capabilities returns the features supported by the driver.
func (d *FirecrackerDriverPlugin) Capabilities() (*drivers.Capabilities, error) {
	return capabilities, nil
}

// Fingerprint returns a channel that will be used to send health information
// and other driver specific node attributes.
func (d *FirecrackerDriverPlugin) Fingerprint(ctx context.Context) (<-chan *drivers.Fingerprint, error) {
	ch := make(chan *drivers.Fingerprint)
	go d.handleFingerprint(ctx, ch)
	return ch, nil
}

// handleFingerprint manages the channel and the flow of fingerprint data.
func (d *FirecrackerDriverPlugin) handleFingerprint(ctx context.Context, ch chan<- *drivers.Fingerprint) {
	defer close(ch)

	// Nomad expects the initial fingerprint to be sent immediately
	ticker := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// after the initial fingerprint we can set the proper fingerprint
			// period
			ticker.Reset(fingerprintPeriod)
			ch <- d.buildFingerprint()
		}
	}
}

// buildFingerprint returns the driver's fingerprint data
func (d *FirecrackerDriverPlugin) buildFingerprint() *drivers.Fingerprint {
	fp := &drivers.Fingerprint{
		Attributes:        map[string]*structs.Attribute{},
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}

	// simple health check: ensure the configured Firecracker binary exists and
	// optionally report its version.  this mirrors the previous shell-based
	// example but uses the actual exec_file we need anyway.
	if d.config == nil || d.config.Jailer == nil || d.config.Jailer.ExecFile == "" {
		// nothing to verify, just return healthy
		return fp
	}

	bin := d.config.Jailer.ExecFile
	if _, err := os.Stat(bin); err != nil {
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = fmt.Sprintf("firecracker binary %s not found", bin)
		return fp
	}

	// try to query version; ignore errors but log a warning
	version := utils.QueryVersion(bin)
	if version != "" {
		fp.Attributes["driver.firecracker.version"] = structs.NewStringAttribute(version)
	}

	return fp
}

// StartTask returns a task handle and a driver network if necessary.
func (d *FirecrackerDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to decode driver config: %v", err)
	}

	// run basic semantic validation on the task configuration.  this checks
	// for required fields and delegates to individual block validators.
	if err := driverConfig.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid task configuration: %v", err)
	}

	// build and serialize the Firecracker VM configuration.  the vm package
	// encapsulates both SDK validation and the boot-source sanity check.
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
	// keep a debug copy of the generated JSON in the log (may be large)
	d.logger.Debug("generated vm configuration", "path", configPath, "json", string(jsonData))

	// no per-task jailer settings; configuration is global to the plugin
	d.logger.Info("starting task", "driver_cfg", hclog.Fmt("%+v", driverConfig))
	if len(driverConfig.NetworkInterfaces) > 0 {
		d.logger.Debug("network configuration", "network", driverConfig.NetworkInterfaces)
	}
	handle := drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	// TODO: implement driver specific mechanism to start the task.
	//
	// Once the task is started you will need to store any relevant runtime
	// information in a taskHandle and TaskState. The taskHandle will be
	// stored in-memory in the plugin and will be used to interact with the
	// task.
	//
	// The TaskState will be returned to the Nomad client inside a
	// drivers.TaskHandle instance. This TaskHandle will be sent back to plugin
	// if the task ever needs to be recovered, so the TaskState should contain
	// enough information to handle that.
	//
	// In the example below we use an executor to fork a process to run our
	// greeter. The executor is then stored in the handle so we can access it
	// later and the the plugin.Client is used to generate a reattach
	// configuration that can be used to recover communication with the task.
	executorConfig := &executor.ExecutorConfig{
		LogFile:  filepath.Join(cfg.TaskDir().Dir, "executor.out"),
		LogLevel: "debug",
	}

	exec, pluginClient, err := executor.CreateExecutor(d.logger, d.nomadConfig, executorConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create executor: %v", err)
	}

	// build the command we will run under Nomad's executor.  the driver is
	// expected to launch Firecracker via the jailer binary so that we get the
	// additional namespace and privilege setup; the configuration has already
	// been validated and defaulted earlier in SetConfig.
	// the plugin config validation already guarantees the jailer block
	// exists, but check once more to avoid nil dereference in tests or
	// accidental invocations without prior SetConfig.
	if d.config == nil || d.config.Jailer == nil {
		pluginClient.Kill()
		return nil, nil, errors.New("jailer configuration missing")
	}

	jConfig := d.config.Jailer
	// build dynamic parameters based on Nomad task state
	params := &jailer.BuildParams{}

	// if the job requested a specific user, attempt to resolve it to
	// numeric ids.  this is best-effort; failures are logged but do not
	// prevent the task from starting because the plugin config may already
	// specify uid/gid or root is acceptable.
	if cfg.User != "" {
		if uid, gid, err := jailer.ResolveUserIDs(cfg.User); err != nil {
			d.logger.Warn("failed to resolve task user for jailer", "user", cfg.User, "err", err)
		} else {
			params.UID = uid
			params.GID = gid
		}
	}

	// pass the network namespace path if one was provided by the
	// driver network manager
	if cfg.NetworkIsolation != nil && cfg.NetworkIsolation.Path != "" {
		params.NetNS = cfg.NetworkIsolation.Path
	}

	// let the jailer builder handle appending firecracker arguments for us
	jArgs, err := jConfig.BuildArgs(cfg.TaskDir().Dir, params, "--config-file", configPath)
	if err != nil {
		pluginClient.Kill()
		return nil, nil, fmt.Errorf("invalid jailer configuration: %v", err)
	}
	// purposefully use a short declaration here; execCmd is a local variable
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
		socketPath:   filepath.Join(cfg.TaskDir().Dir, "jailer/root/run/firecracker.socket"),
	}

	driverState := TaskState{
		ReattachConfig: structs.ReattachConfigFromGoPlugin(pluginClient.ReattachConfig()),
		Pid:            ps.Pid,
		TaskConfig:     cfg,
		StartedAt:      h.startedAt,
		SocketPath:     h.socketPath,
	}

	if err := handle.SetDriverState(&driverState); err != nil {
		return nil, nil, fmt.Errorf("failed to set driver state: %v", err)
	}

	d.tasks.Set(cfg.ID, h)
	go h.run()
	return handle, nil, nil
}

// RecoverTask recreates the in-memory state of a task from a TaskHandle.
func (d *FirecrackerDriverPlugin) RecoverTask(handle *drivers.TaskHandle) error {
	if handle == nil {
		return errors.New("error: handle cannot be nil")
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
		if err := driverConfig.NetworkInterfaces.validate(nil); err != nil {
			return fmt.Errorf("invalid network configuration on recover: %v", err)
		}
	}

	// TODO: implement driver specific logic to recover a task.
	//
	// Recovering a task involves recreating and storing a taskHandle as if the
	// task was just started.
	//
	// In the example below we use the executor to re-attach to the process
	// that was created when the task first started.
	plugRC, err := structs.ReattachConfigToGoPlugin(taskState.ReattachConfig)
	if err != nil {
		return fmt.Errorf("failed to build ReattachConfig from taskConfig state: %v", err)
	}

	execImpl, pluginClient, err := executor.ReattachToExecutor(plugRC, d.logger, d.nomadConfig.Topology.Compute())
	if err != nil {
		return fmt.Errorf("failed to reattach to executor: %v", err)
	}

	h := &taskHandle{
		exec:         execImpl,
		pid:          taskState.Pid,
		pluginClient: pluginClient,
		taskConfig:   taskState.TaskConfig,
		procState:    drivers.TaskStateRunning,
		startedAt:    taskState.StartedAt,
		exitResult:   &drivers.ExitResult{},
		socketPath:   taskState.SocketPath,
		logger:       d.logger,
	}

	d.tasks.Set(taskState.TaskConfig.ID, h)

	go h.run()
	return nil
}

// WaitTask returns a channel used to notify Nomad when a task exits.
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

	// TODO: implement driver specific logic to notify Nomad the task has been
	// completed and what was the exit result.
	//
	// When a result is sent in the result channel Nomad will stop the task and
	// emit an event that an operator can use to get an insight on why the task
	// stopped.
	//
	// In the example below we block and wait until the executor finishes
	// running, at which point we send the exit code and signal in the result
	// channel.
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

// StopTask stops a running task with the given signal and within the timeout window.
func (d *FirecrackerDriverPlugin) StopTask(taskID string, timeout time.Duration, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	// Attempt graceful shutdown via HTTP API with timeout context
	if handle.socketPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		c := client.New(handle.socketPath)
		if err := c.Shutdown(ctx); err != nil {
			d.logger.Warn("graceful shutdown failed, will force kill", "err", err)
		} else {
			// Graceful shutdown succeeded
			return nil
		}
	}

	// Fallback to executor shutdown if HTTP shutdown failed or socket unavailable
	if err := handle.exec.Shutdown(signal, timeout); err != nil {
		if handle.pluginClient.Exited() {
			return nil
		}
		return fmt.Errorf("executor Shutdown failed: %v", err)
	}

	return nil
}

// DestroyTask cleans up and removes a task that has terminated.
func (d *FirecrackerDriverPlugin) DestroyTask(taskID string, force bool) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	if handle.IsRunning() && !force {
		return errors.New("cannot destroy running task")
	}

	// TODO: implement driver specific logic to destroy a complete task.
	//
	// Destroying a task includes removing any resources used by task and any
	// local references in the plugin. If force is set to true the task should
	// be destroyed even if it's currently running.
	//
	// In the example below we use the executor to force shutdown the task
	// (timeout equals 0).
	if !handle.pluginClient.Exited() {
		if err := handle.exec.Shutdown("", 0); err != nil {
			handle.logger.Error("destroying executor failed", "err", err)
		}

		handle.pluginClient.Kill()
	}

	d.tasks.Delete(taskID)
	return nil
}

// InspectTask returns detailed status information for the referenced taskID.
func (d *FirecrackerDriverPlugin) InspectTask(taskID string) (*drivers.TaskStatus, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	return handle.TaskStatus(), nil
}

// TaskStats returns a channel which the driver should send stats to at the given interval.
func (d *FirecrackerDriverPlugin) TaskStats(ctx context.Context, taskID string, interval time.Duration) (<-chan *drivers.TaskResourceUsage, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	// TODO: implement driver specific logic to send task stats.
	//
	// This function returns a channel that Nomad will use to listen for task
	// stats (e.g., CPU and memory usage) in a given interval. It should send
	// stats until the context is canceled or the task stops running.
	//
	// In the example below we use the Stats function provided by the executor,
	// but you can build a set of functions similar to the fingerprint process.
	return handle.exec.Stats(ctx, interval)
}

// TaskEvents returns a channel that the plugin can use to emit task related events.
func (d *FirecrackerDriverPlugin) TaskEvents(ctx context.Context) (<-chan *drivers.TaskEvent, error) {
	return d.eventer.TaskEvents(ctx)
}

// SignalTask forwards a signal to a task.
// This is an optional capability.
func (d *FirecrackerDriverPlugin) SignalTask(taskID string, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	// TODO: implement driver specific signal handling logic.
	//
	// The given signal must be forwarded to the target taskID. If this plugin
	// doesn't support receiving signals (capability SendSignals is set to
	// false) you can just return nil.
	sig := os.Interrupt
	if s, ok := signals.SignalLookup[signal]; ok {
		sig = s
	} else {
		d.logger.Warn("unknown signal to send to task, using SIGINT instead", "signal", signal, "task_id", handle.taskConfig.ID)

	}
	return handle.exec.Signal(sig)
}

// ExecTask returns the result of executing the given command inside a task.
// This is an optional capability.
func (d *FirecrackerDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	// TODO: implement driver specific logic to execute commands in a task.
	return nil, errors.New("This driver does not support exec")
}
