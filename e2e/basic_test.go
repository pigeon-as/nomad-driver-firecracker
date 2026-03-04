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

// allJobs lists every e2e job name, used for blanket cleanup between runs.
var allJobs = []string{"basic", "bridge", "echo", "mmds", "snapshot", "volume-mount"}

func setup(t *testing.T) context.Context {
	t.Helper()
	cleanNomad()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(func() {
		cancel()
		cleanNomad()
	})
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

// execCmd runs a command, ignoring errors (best-effort cleanup).
func execCmd(ctx context.Context, command string, args ...string) {
	exec.CommandContext(ctx, command, args...).CombinedOutput() //nolint:errcheck
}

func purge(job string) func() {
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		execCmd(ctx, "nomad", "job", "stop", "-purge", job)
	}
}

// cleanNomad purges all known e2e jobs and host volumes. Best-effort;
// errors are silently ignored so it is safe to call from cleanup paths.
// Uses its own context so it works even when the test context has expired.
func cleanNomad() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop all known jobs first.
	for _, job := range allJobs {
		execCmd(ctx, "nomad", "job", "stop", "-purge", job)
	}
	// Give Nomad a moment to release volumes.
	time.Sleep(2 * time.Second)

	// Delete any leftover host volumes.
	out, err := exec.CommandContext(ctx, "nomad", "volume", "status",
		"-type", "host", "-verbose").CombinedOutput()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[1] != "ID" {
				execCmd(ctx, "nomad", "volume", "delete", "-type", "host", fields[1])
			}
		}
	}

	execCmd(ctx, "nomad", "system", "gc")
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
	defer purge("basic")()

	run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")

	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "basic"))

	stopOut := run(t, ctx, "nomad", "job", "stop", "basic")
	must.StrContains(t, stopOut, `finished with status "complete"`)

	must.RegexMatch(t, deadRe, run(t, ctx, "nomad", "job", "status", "basic"))
}

func TestBridgeAllocGetsIP(t *testing.T) {
	ctx := setup(t)
	defer purge("bridge")()

	run(t, ctx, "nomad", "job", "run", "./jobs/bridge.hcl")
	waitForRunning(t, ctx, "bridge")

	id := allocID(t, ctx, "bridge")
	must.RegexMatch(t, ipRe, run(t, ctx, "nomad", "alloc", "status", id))
}

func TestBasicStdout(t *testing.T) {
	ctx := setup(t)
	defer purge("basic")()

	run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")
	waitForRunning(t, ctx, "basic")

	logs := waitForLogs(t, ctx, allocID(t, ctx, "basic"), "firecracker", "Linux")
	must.StrContains(t, logs, "Linux")
}

func TestMMDSMetadata(t *testing.T) {
	ctx := setup(t)
	defer purge("mmds")()

	run(t, ctx, "nomad", "job", "run", "./jobs/mmds.hcl")
	waitForRunning(t, ctx, "mmds")
	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "mmds"))
}

func TestSnapshotRestart(t *testing.T) {
	ctx := setup(t)
	defer purge("snapshot")()

	run(t, ctx, "nomad", "job", "run", "-detach", "./jobs/snapshot.hcl")
	waitForRunning(t, ctx, "snapshot")

	id := allocID(t, ctx, "snapshot")
	run(t, ctx, "nomad", "alloc", "restart", id)

	time.Sleep(2 * time.Second)
	waitForRunning(t, ctx, "snapshot")
	must.RegexMatch(t, runningRe, run(t, ctx, "nomad", "job", "status", "snapshot"))
}

func TestHTTPEcho(t *testing.T) {
	cleanNomad()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	t.Cleanup(func() {
		cancel()
		cleanNomad()
	})
	defer purge("echo")()

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
