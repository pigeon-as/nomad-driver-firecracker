package firecracker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

var versionRegex = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

func (d *FirecrackerDriverPlugin) handleFingerprint(ctx context.Context, ch chan<- *drivers.Fingerprint) {
	defer close(ch)
	ticker := time.NewTimer(0)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			ticker.Reset(fingerprintPeriod)
			ch <- d.buildFingerprint()
		}
	}
}

func (d *FirecrackerDriverPlugin) buildFingerprint() *drivers.Fingerprint {
	fp := &drivers.Fingerprint{
		Attributes:        map[string]*structs.Attribute{},
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}

	kvmFile, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		fp.Health = drivers.HealthStateUndetected
		if os.IsNotExist(err) {
			fp.HealthDescription = "/dev/kvm not available: KVM is required for Firecracker"
		} else if os.IsPermission(err) {
			fp.HealthDescription = "cannot access /dev/kvm: permission denied; ensure the Nomad client user is in the kvm group"
		} else {
			fp.HealthDescription = fmt.Sprintf("error accessing /dev/kvm: %v", err)
		}
		return fp
	}
	kvmFile.Close()

	if d.config == nil || d.config.Jailer == nil || d.config.Jailer.ExecFile == "" {
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = "firecracker binary not configured"
		return fp
	}

	jailerBin := d.config.Jailer.Bin()
	if _, err := exec.LookPath(jailerBin); err != nil {
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = fmt.Sprintf("jailer binary %q not found: %v", jailerBin, err)
		return fp
	}

	fcBin := d.config.Jailer.ExecFile
	fcPath, err := exec.LookPath(fcBin)
	if err != nil {
		fp.Health = drivers.HealthStateUndetected
		fp.HealthDescription = fmt.Sprintf("firecracker binary %q not found: %v", fcBin, err)
		return fp
	}

	version := queryVersion(fcPath)
	if version != "" {
		fp.Attributes["driver.firecracker.version"] = structs.NewStringAttribute(version)
	}

	return fp
}

func queryVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return versionRegex.FindString(string(out))
}
