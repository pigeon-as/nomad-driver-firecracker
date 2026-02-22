// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

package firecracker

import (
	"context"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/consul-template/signals"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/nomad/drivers/shared/executor"
	"github.com/hashicorp/nomad/plugins/drivers"

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/client"
)

type taskHandle struct {
	stateLock sync.RWMutex

	logger       hclog.Logger
	exec         executor.Executor
	pluginClient *plugin.Client
	taskConfig   *drivers.TaskConfig
	procState    drivers.TaskState
	startedAt    time.Time
	completedAt  time.Time
	exitResult   *drivers.ExitResult
	pid          int
	socketPath   string
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

	// Use background context so the wait can outlive the driver
	// if needed (e.g., during driver hot reload)
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

// forwardSignal forwards a signal to the Firecracker VMM process.
// Signals SIGTERM and SIGINT trigger graceful VM shutdown via Ctrl+Alt+Del.
// Other signals are forwarded to the Firecracker process for handling.
func (h *taskHandle) forwardSignal(ctx context.Context, signalName string) error {
	// Parse the signal
	sig := os.Interrupt
	if s, ok := signals.SignalLookup[signalName]; ok {
		sig = s
	} else {
		h.logger.Warn("unknown signal to forward to firecracker, using SIGINT", "signal", signalName, "task_id", h.taskConfig.ID)
	}

	// For graceful shutdown signals, attempt Ctrl+Alt+Del first via Firecracker API
	if sig == syscall.SIGTERM || sig == syscall.SIGINT {
		if h.socketPath == "" {
			h.logger.Debug("socket path not available, cannot attempt graceful shutdown via ctrl+alt+del", "task_id", h.taskConfig.ID)
		} else {
			c := client.New(h.socketPath)
			if err := c.SendCtrlAltDel(ctx); err != nil {
				h.logger.Debug("graceful shutdown via ctrl+alt+del failed, will forward signal to process", "signal", signalName, "err", err)
				// Fall through to send signal to executor
			} else {
				h.logger.Info("graceful shutdown initiated via ctrl+alt+del", "task_id", h.taskConfig.ID)
				return nil
			}
		}
	}

	// Forward the signal to the executor/VMM process
	h.logger.Debug("forwarding signal to firecracker process", "signal", signalName, "task_id", h.taskConfig.ID)
	return h.exec.Signal(sig)
}
