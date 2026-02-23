// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build e2e

// Run these tests manually by setting the e2e tag when running go test, e.g.
//
//	➜ go test -tags=e2e -v ./e2e
//
// For editing set: export GOFLAGS='--tags=e2e'

package e2e

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

const timeout = 30 * time.Second

func pause() {
	time.Sleep(2 * time.Second)
	if ci := os.Getenv("CI"); ci != "" {
		time.Sleep(500 * time.Millisecond)
	}
}

func setup(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(func() {
		run(t, ctx, "nomad", "system", "gc")
		cancel()
	})
	pause()
	return ctx
}

func run(t *testing.T, ctx context.Context, command string, args ...string) string {
	t.Logf("RUN '%s %s'", command, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, command, args...)
	b, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(b))
	if err != nil {
		t.Log("ERR:", err)
		t.Log("OUT:", output)
		t.FailNow()
	}
	return output
}

func purge(t *testing.T, ctx context.Context, job string) func() {
	return func() {
		t.Log("STOP", job)
		cmd := exec.CommandContext(ctx, "nomad", "job", "stop", "-purge", job)
		b, err := cmd.CombinedOutput()
		output := strings.TrimSpace(string(b))
		if err != nil {
			t.Log("ERR:", err)
			t.Log("OUT:", output)
		}
		pause()
	}
}

var (
	// runningRe is a regex for checking the "running" status of a job.
	runningRe = regexp.MustCompile(`Status\s+=\s+running`)
	// deadRe is a regex for checking the "dead" status of a job.
	deadRe = regexp.MustCompile(`Status\s+=\s+dead`)
	// allocIDRe extracts allocation IDs from job status output.
	allocIDRe = regexp.MustCompile(`([a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12})`)
)

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

// waitForLogs polls until the task's stdout contains substr or the timeout expires.
func waitForLogs(t *testing.T, ctx context.Context, allocID, task, substr string) string {
	t.Helper()
	deadline := time.After(timeout)
	var logs string
	for {
		out, err := exec.CommandContext(ctx, "nomad", "alloc", "logs", allocID, task).CombinedOutput()
		logs = strings.TrimSpace(string(out))
		if err == nil && strings.Contains(logs, substr) {
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

// TestPluginStarts verifies the Firecracker driver plugin starts and is healthy.
func TestPluginStarts(t *testing.T) {
	ctx := setup(t)

	// Can connect to nomad
	jobs := run(t, ctx, "nomad", "job", "status")
	must.StrContains(t, jobs, "No running jobs")

	// Firecracker plugin is present and healthy
	status := run(t, ctx, "nomad", "node", "status", "-self", "-verbose")
	fcRe := regexp.MustCompile(`firecracker\s+true\s+true\s+Healthy`)
	must.RegexMatch(t, fcRe, status)
}

// TestBasic_Lifecycle verifies a firecracker VM can be started and stopped.
func TestBasic_Lifecycle(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "basic")()

	_ = run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")

	// Verify job becomes running
	jobStatus := run(t, ctx, "nomad", "job", "status", "basic")
	must.RegexMatch(t, runningRe, jobStatus)

	// Stop the job gracefully
	stopOutput := run(t, ctx, "nomad", "job", "stop", "basic")
	must.StrContains(t, stopOutput, `finished with status "complete"`)

	// Verify job is stopped
	stopStatus := run(t, ctx, "nomad", "job", "status", "basic")
	must.RegexMatch(t, deadRe, stopStatus)
}

// TestBridge_AllocGetsIP verifies that a VM in bridge network mode gets an
// allocated IP address, confirming TAP + TC redirect setup works.
func TestBridge_AllocGetsIP(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "bridge")()

	_ = run(t, ctx, "nomad", "job", "run", "./jobs/bridge.hcl")
	waitForRunning(t, ctx, "bridge")

	allocs := run(t, ctx, "nomad", "job", "allocs", "-json", "bridge")
	allocID := regexp.MustCompile(`"ID"\s*:\s*"([^"]+)"`).FindStringSubmatch(allocs)
	must.SliceNotEmpty(t, allocID)

	// Check that the allocation has a network status with an IP address.
	allocStatus := run(t, ctx, "nomad", "alloc", "status", allocID[1])
	ipRe := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+`)
	must.RegexMatch(t, ipRe, allocStatus)
}

// TestBasic_Stdout verifies that a VM writes kernel boot output to stdout.
func TestBasic_Stdout(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "basic")()

	_ = run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")
	waitForRunning(t, ctx, "basic")

	allocs := run(t, ctx, "nomad", "job", "allocs", "-json", "basic")
	allocID := regexp.MustCompile(`"ID"\s*:\s*"([^"]+)"`).FindStringSubmatch(allocs)
	must.SliceNotEmpty(t, allocID)

	logs := waitForLogs(t, ctx, allocID[1], "firecracker", "Linux")
	must.StrContains(t, logs, "Linux")
}

// TestMMDS_Metadata verifies that a VM boots successfully with MMDS metadata
// configured. The metadata is set via the Firecracker API after the
// Firecracker process starts and its API socket is ready.
func TestMMDS_Metadata(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "mmds")()

	_ = run(t, ctx, "nomad", "job", "run", "./jobs/mmds.hcl")
	waitForRunning(t, ctx, "mmds")

	// Verify job reached running state — this confirms MMDS config was
	// accepted by Firecracker and PutMmds succeeded without errors.
	jobStatus := run(t, ctx, "nomad", "job", "status", "mmds")
	must.RegexMatch(t, runningRe, jobStatus)
}

// TestSnapshot_Restart verifies the snapshot boot feature by cold booting
// a VM, restarting the allocation (which triggers snapshot save + restore),
// and confirming the task comes back up running.
func TestSnapshot_Restart(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "snapshot")()

	// Cold boot — use -detach because service job deployment monitoring
	// would block until healthy (or until the context deadline).
	_ = run(t, ctx, "nomad", "job", "run", "-detach", "./jobs/snapshot.hcl")
	waitForRunning(t, ctx, "snapshot")

	// Extract allocation ID.
	allocs := run(t, ctx, "nomad", "job", "allocs", "-json", "snapshot")
	allocID := regexp.MustCompile(`"ID"\s*:\s*"([^"]+)"`).FindStringSubmatch(allocs)
	must.SliceNotEmpty(t, allocID)

	// Restart the allocation — triggers StopTask (snapshot save) followed
	// by StartTask (snapshot restore) within the same allocation.
	_ = run(t, ctx, "nomad", "alloc", "restart", allocID[1])

	// Wait for the task to come back up after snapshot restore.
	pause()
	waitForRunning(t, ctx, "snapshot")

	jobStatus := run(t, ctx, "nomad", "job", "status", "snapshot")
	must.RegexMatch(t, runningRe, jobStatus)
}
