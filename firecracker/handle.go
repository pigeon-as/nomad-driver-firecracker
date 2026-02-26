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

	"github.com/pigeon-as/nomad-driver-firecracker/firecracker/guestapi"
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

// forwardSignal delivers a signal to the guest workload.
//
// 3-tier approach: (1) vsock guest agent — arch-independent, any signal.
// (2) Ctrl+Alt+Del — x86_64 only, SIGTERM/SIGINT only.
// (3) executor process signal — last resort.
func (h *taskHandle) forwardSignal(ctx context.Context, signalName string, gc *guestapi.Client) error {
	s, ok := signals.SignalLookup[signalName]
	if !ok {
		return fmt.Errorf("unknown signal %q", signalName)
	}
	sig, ok := s.(syscall.Signal)
	if !ok {
		return fmt.Errorf("unsupported signal type %T", s)
	}

	// Tier 1: vsock guest agent (any architecture, any signal).
	if gc != nil {
		if err := gc.Signal(ctx, int(sig)); err != nil {
			h.logger.Debug("vsock signal failed, trying fallback", "signal", signalName, "err", err)
		} else {
			h.logger.Info("signal delivered via vsock", "signal", signalName, "task_id", h.taskConfig.ID)
			return nil
		}
	}

	// Tier 2: Ctrl+Alt+Del (x86_64 only, SIGTERM/SIGINT only).
	if (sig == syscall.SIGTERM || sig == syscall.SIGINT) && h.socketPath != "" {
		c := machine.NewClient(h.socketPath)
		if err := c.SendCtrlAltDel(ctx); err != nil {
			h.logger.Debug("ctrl+alt+del failed, forwarding to executor", "signal", signalName, "err", err)
		} else {
			h.logger.Info("graceful shutdown initiated via ctrl+alt+del", "task_id", h.taskConfig.ID)
			return nil
		}
	}

	// Tier 3: executor process signal.
	return h.exec.Signal(sig)
}
