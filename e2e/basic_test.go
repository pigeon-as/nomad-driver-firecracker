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
	if ci := os.Getenv("CI"); ci == "" {
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(2 * time.Second)
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
)

// TestPluginStarts verifies the Firecracker driver plugin starts and is healthy.
func TestPluginStarts(t *testing.T) {
	ctx := setup(t)

	// Can connect to nomad
	jobs := run(t, ctx, "nomad", "job", "status")
	must.Eq(t, "No running jobs", jobs)

	// Firecracker plugin is present and healthy
	status := run(t, ctx, "nomad", "node", "status", "-self", "-verbose")
	fcRe := regexp.MustCompile(`firecracker\s+true\s+true\s+Healthy`)
	must.RegexMatch(t, fcRe, status)
}

// TestBasic_Echo verifies a simple firecracker task runs and produces output.
// This tests VM launch, guest OS execution, and log capture.
func TestBasic_Echo(t *testing.T) {
	ctx := setup(t)
	defer purge(t, ctx, "echo")()

	// Run a simple echo job
	_ = run(t, ctx, "nomad", "job", "run", "./e2e/jobs/echo.hcl")

	// Verify job becomes running
	jobStatus := run(t, ctx, "nomad", "job", "status", "echo")
	must.RegexMatch(t, runningRe, jobStatus)

	// Stop the job gracefully
	stopOutput := run(t, ctx, "nomad", "job", "stop", "echo")
	must.StrContains(t, stopOutput, `finished with status "complete"`)

	// Verify job is stopped
	stopStatus := run(t, ctx, "nomad", "job", "status", "echo")
	must.RegexMatch(t, deadRe, stopStatus)
}
