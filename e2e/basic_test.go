// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build e2e

// Run: make e2e (requires a running nomad dev agent with the plugin)

package e2e

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

const timeout = 30 * time.Second

var (
	runningRe = regexp.MustCompile(`Status\s+=\s+running`)
	deadRe    = regexp.MustCompile(`Status\s+=\s+dead`)
	idRe      = regexp.MustCompile(`"ID"\s*:\s*"([^"]+)"`)
	ipRe      = regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)
)

func setup(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(func() {
		run(t, ctx, "nomad", "system", "gc")
		cancel()
	})
	time.Sleep(2 * time.Second)
	return ctx
}

func run(t *testing.T, ctx context.Context, command string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, command, args...)
	b, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(b))
	if err != nil {
		t.Fatalf("'%s %s' failed: %v\n%s", command, strings.Join(args, " "), err, output)
	}
	return output
}

func purge(t *testing.T, ctx context.Context, job string) func() {
	return func() {
		cmd := exec.CommandContext(ctx, "nomad", "job", "stop", "-purge", job)
		cmd.CombinedOutput()
		time.Sleep(2 * time.Second)
	}
}

func waitForRunning(t *testing.T, ctx context.Context, job string) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job %q to start", job)
		default:
			out := run(t, ctx, "nomad", "job", "status", job)
			if runningRe.MatchString(out) || deadRe.MatchString(out) {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func waitForLogs(t *testing.T, ctx context.Context, allocID, task, substr string) string {
	t.Helper()
	deadline := time.After(timeout)
	var logs string
	for {
		out, _ := exec.CommandContext(ctx, "nomad", "alloc", "logs", allocID, task).CombinedOutput()
		logs = strings.TrimSpace(string(out))
		if strings.Contains(logs, substr) {
			return logs
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %q in task %q logs:\n%s", substr, task, logs)
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func allocID(t *testing.T, ctx context.Context, job string) string {
	t.Helper()
	allocs := run(t, ctx, "nomad", "job", "allocs", "-json", job)
	m := idRe.FindStringSubmatch(allocs)
	must.SliceNotEmpty(t, m)
	return m[1]
}

func waitForService(t *testing.T, ctx context.Context, name string) string {
	t.Helper()
	addrRe := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+:\d+)`)
	for {
		cmd := exec.CommandContext(ctx, "nomad", "service", "info", name)
		out, err := cmd.CombinedOutput()
		if err == nil {
			if m := addrRe.FindStringSubmatch(string(out)); len(m) > 0 {
				return m[1]
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for service %q", name)
		default:
			time.Sleep(2 * time.Second)
		}
	}
}

// --- tests ---

func TestPluginStarts(t *testing.T) {
	ctx := setup(t)

	jobs := run(t, ctx, "nomad", "job", "status")
	must.StrContains(t, jobs, "No running jobs")

	status := run(t, ctx, "nomad", "node", "status", "-self", "-verbose")
	must.RegexMatch(t, regexp.MustCompile(`firecracker\s+true\s+true\s+Healthy`), status)
}

func TestBasicLifecycle(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "basic")()

	run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")

	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "basic"))

	stopOut := run(t, ctx, "nomad", "job", "stop", "basic")
	must.StrContains(t, stopOut, `finished with status "complete"`)

	must.RegexMatch(t, deadRe, run(t, ctx, "nomad", "job", "status", "basic"))
}

func TestBridgeAllocGetsIP(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "bridge")()

	run(t, ctx, "nomad", "job", "run", "./jobs/bridge.hcl")
	waitForRunning(t, ctx, "bridge")

	id := allocID(t, ctx, "bridge")
	must.RegexMatch(t, ipRe, run(t, ctx, "nomad", "alloc", "status", id))
}

func TestBasicStdout(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "basic")()

	run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")
	waitForRunning(t, ctx, "basic")

	logs := waitForLogs(t, ctx, allocID(t, ctx, "basic"), "firecracker", "Linux")
	must.StrContains(t, logs, "Linux")
}

func TestMMDSMetadata(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "mmds")()

	run(t, ctx, "nomad", "job", "run", "./jobs/mmds.hcl")
	waitForRunning(t, ctx, "mmds")
	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "mmds"))
}

func TestSnapshotRestart(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "snapshot")()

	run(t, ctx, "nomad", "job", "run", "-detach", "./jobs/snapshot.hcl")
	waitForRunning(t, ctx, "snapshot")

	id := allocID(t, ctx, "snapshot")
	run(t, ctx, "nomad", "alloc", "restart", id)

	time.Sleep(2 * time.Second)
	waitForRunning(t, ctx, "snapshot")
	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "snapshot"))
}

func TestHTTPEcho(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(func() {
		run(t, ctx, "nomad", "system", "gc")
		cancel()
	})
	defer purge(t, ctx, "echo")()

	run(t, ctx, "nomad", "job", "run", "-detach", "./jobs/echo.hcl")
	waitForRunning(t, ctx, "echo")
	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "echo"))

	addr := waitForService(t, ctx, "http-echo")

	// Retry curl — VM needs time to boot and start http-echo.
	var resp string
	for {
		cmd := exec.CommandContext(ctx, "curl", "-sf", "http://"+addr)
		out, err := cmd.CombinedOutput()
		if err == nil {
			resp = strings.TrimSpace(string(out))
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for http-echo at %s", addr)
		default:
			time.Sleep(2 * time.Second)
		}
	}
	must.StrContains(t, resp, "hello")
}
