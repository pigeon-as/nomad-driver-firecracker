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

func waitForLogs(t *testing.T, ctx context.Context, allocID, task string) string {
	t.Helper()
	deadline := time.After(timeout)
	for {
		cmd := exec.CommandContext(ctx, "nomad", "alloc", "logs", allocID, task)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for logs from task %q: %s", task, string(out))
		default:
			time.Sleep(2 * time.Second)
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

// TestBasic_Stdout verifies that a VM writes kernel boot output to stdout.
func TestBasic_Stdout(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "basic")()

	_ = run(t, ctx, "nomad", "job", "run", "./jobs/basic.hcl")
	waitForRunning(t, ctx, "basic")

	allocs := run(t, ctx, "nomad", "job", "allocs", "-json", "basic")
	allocID := regexp.MustCompile(`"ID"\s*:\s*"([^"]+)"`).FindStringSubmatch(allocs)
	must.SliceNotEmpty(t, allocID)

	logs := waitForLogs(t, ctx, allocID[1], "firecracker")
	must.StrContains(t, logs, "Linux")
}
