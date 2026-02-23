// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package firecracker

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/consul-template/signals"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/nomad/drivers/shared/executor"
	"github.com/hashicorp/nomad/plugins/drivers"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/machine"
)

type taskHandle struct {
	exec         executor.Executor
	pid          int
	pluginClient *plugin.Client
	logger       hclog.Logger
	socketPath   string

	stateLock sync.RWMutex

	taskConfig  *drivers.TaskConfig
	procState   drivers.TaskState
	startedAt   time.Time
	completedAt time.Time
	exitResult  *drivers.ExitResult
}

func (h *taskHandle) TaskStatus() *drivers.TaskStatus {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()

	return &drivers.TaskStatus{
		ID:          h.taskConfig.ID,
		Name:        h.taskConfig.Name,
		State:       h.procState,
		StartedAt:   h.startedAt,
		CompletedAt: h.completedAt,
		ExitResult:  h.exitResult,
		DriverAttributes: map[string]string{
			"pid": strconv.Itoa(h.pid),
		},
	}
}

func (h *taskHandle) IsRunning() bool {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()
	return h.procState == drivers.TaskStateRunning
}

func (h *taskHandle) run() {
	h.stateLock.Lock()
	if h.exitResult == nil {
		h.exitResult = &drivers.ExitResult{}
	}
	h.stateLock.Unlock()

	ps, err := h.exec.Wait(context.Background())

	h.stateLock.Lock()
	defer h.stateLock.Unlock()

	if err != nil {
		h.exitResult.Err = err
		h.procState = drivers.TaskStateUnknown
		h.completedAt = time.Now()
		return
	}
	h.procState = drivers.TaskStateExited
	h.exitResult.ExitCode = ps.ExitCode
	h.exitResult.Signal = ps.Signal
	h.completedAt = ps.Time
}

// forwardSignal sends SIGTERM/SIGINT as Ctrl+Alt+Del via the Firecracker API.
// Other signals are forwarded directly to the executor process.
func (h *taskHandle) forwardSignal(ctx context.Context, signalName string) error {
	sig, ok := signals.SignalLookup[signalName]
	if !ok {
		return fmt.Errorf("unknown signal %q", signalName)
	}

	// For SIGTERM/SIGINT, attempt graceful shutdown via Firecracker API
	if sig == syscall.SIGTERM || sig == syscall.SIGINT {
		if h.socketPath == "" {
			h.logger.Debug("socket path not available, cannot attempt graceful shutdown via ctrl+alt+del", "task_id", h.taskConfig.ID)
		} else {
			c := machine.NewClient(h.socketPath)
			if err := c.SendCtrlAltDel(ctx); err != nil {
				h.logger.Debug("ctrl+alt+del failed, forwarding signal", "signal", signalName, "err", err)
			} else {
				h.logger.Info("graceful shutdown initiated via ctrl+alt+del", "task_id", h.taskConfig.ID)
				return nil
			}
		}
	}

	return h.exec.Signal(sig)
}
