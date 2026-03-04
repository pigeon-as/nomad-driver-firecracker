// Copyright IBM Corp. 2019, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build e2e

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

var volIDRe = regexp.MustCompile(`(?i)with ID\s+(\S+)`)

// Must match the parameters in e2e/volumes/*.hcl.
const (
	lvmVG       = "e2e-vg"
	lvmPool     = "thinpool"
	lvmMountDir = "/tmp/nomad-volumes"
)

// TestVolumeMountPersist validates the full volume mount pipeline:
//
//  1. nomad volume create (mode=block) → LVM plugin creates thin volume
//  2. nomad job run → driver auto-attaches block device as Firecracker drive,
//     injects {DevicePath, MountPath} into MMDS
//  3. pigeon-init mounts /dev/vdX at /data inside the guest
//  4. Guest writes marker file → job completes
//  5. Re-run same job → guest reads marker file written in step 4
//
// Prerequisites:
//   - LVM thin pool on the host (see ROADMAP.md Track B)
//   - nomad-plugin-lvm registered in Nomad agent config
//   - Kernel, initrd, rootfs in /tmp/testdata (make kernel init rootfs)
func TestVolumeMountPersist(t *testing.T) {
	if !lvmAvailable() {
		t.Skip("LVM tools not available or not running as root")
	}

	cleanNomad()
	setupLVM(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(func() {
		cancel()
		cleanNomad()
	})

	// --- Create volume ---
	volOut := run(t, ctx, "nomad", "volume", "create", "./volumes/persistent-block.hcl")
	m := volIDRe.FindStringSubmatch(volOut)
	must.SliceNotEmpty(t, m, must.Sprintf("expected volume ID in output:\n%s", volOut))
	t.Logf("created volume %s", m[1])

	// --- First run: guest sees no marker, writes one ---
	run(t, ctx, "nomad", "job", "run", "./jobs/volume-mount.hcl")
	waitForDead(t, ctx, "volume-mount")

	id1 := allocID(t, ctx, "volume-mount")
	logs1 := waitForLogs(t, ctx, id1, "firecracker", "volume-mount-ok")
	must.True(t, hasOutputLine(logs1, "NO_MARKER"), must.Sprint("expected NO_MARKER as standalone output line"))
	must.StrContains(t, logs1, "volume-mount-ok")
	t.Log("first run: marker written")

	// Purge old alloc so the job can be re-run.
	run(t, ctx, "nomad", "job", "stop", "-purge", "volume-mount")
	time.Sleep(2 * time.Second)
	run(t, ctx, "nomad", "system", "gc")

	// --- Second run: guest reads marker from previous run ---
	run(t, ctx, "nomad", "job", "run", "./jobs/volume-mount.hcl")
	waitForDead(t, ctx, "volume-mount")

	id2 := allocID(t, ctx, "volume-mount")
	logs2 := waitForLogs(t, ctx, id2, "firecracker", "volume-mount-ok")
	// The marker file should exist from the first run.
	must.StrContains(t, logs2, "volume-mount-ok")
	must.False(t, hasOutputLine(logs2, "NO_MARKER"), must.Sprint("NO_MARKER should not appear as standalone output line"))
	t.Log("second run: marker persisted across restarts")
}

// waitForDead polls until the job reaches "dead" status.
func waitForDead(t *testing.T, ctx context.Context, job string) {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job %q to finish", job)
		default:
			out := run(t, ctx, "nomad", "job", "status", job)
			if deadRe.MatchString(out) {
				return
			}
			time.Sleep(2 * time.Second)
		}
	}
}

// --- LVM loopback infra ---

// setupLVM creates a loopback LVM thin pool for volume tests.
// The VG/pool names must match the LVM_* environment variables.
// Registers t.Cleanup to tear everything down.
func setupLVM(t *testing.T) {
	t.Helper()

	// Tear down leftovers from a previous failed run.
	lvmTryShell("vgremove", "--force", lvmVG)
	os.RemoveAll(lvmMountDir)

	f, err := os.CreateTemp("", "fc-lvm-e2e-*.img")
	must.NoError(t, err)
	loopImg := f.Name()
	f.Close()

	lvmShell(t, "truncate", "-s", "1G", loopImg)
	loopDev := lvmShell(t, "losetup", "--find", "--show", loopImg)
	lvmShell(t, "pvcreate", loopDev)
	lvmShell(t, "vgcreate", lvmVG, loopDev)
	lvmShell(t, "lvcreate", "--type", "thin-pool", "--name", lvmPool, "--size", "900M", lvmVG)

	t.Cleanup(func() {
		lvmTryShell("vgremove", "--force", lvmVG)
		lvmTryShell("losetup", "--detach", loopDev)
		os.Remove(loopImg)
		os.RemoveAll(lvmMountDir)
	})
}

func lvmShell(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, s)
	}
	return s
}

func lvmTryShell(name string, args ...string) {
	exec.Command(name, args...).CombinedOutput() //nolint:errcheck
}

// lvmAvailable returns true if LVM tools are on PATH and we're root.
func lvmAvailable() bool {
	_, err := exec.LookPath("lvcreate")
	return err == nil && os.Geteuid() == 0
}

// hasOutputLine returns true if needle appears as a standalone trimmed line in s.
// This avoids false positives from pigeon-init's argv log which embeds the full
// command text (including sentinel strings) inside a structured log line.
func hasOutputLine(s, needle string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == needle {
			return true
		}
	}
	return false
}
