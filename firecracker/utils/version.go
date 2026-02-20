//go:build !windows
// +build !windows

package utils

import (
	"context"
	"regexp"

	fc "github.com/firecracker-microvm/firecracker-go-sdk"
)

// QueryVersion attempts to retrieve the Firecracker binary version.
// It uses the firecracker-go-sdk command builder to invoke the binary with --version
// and returns the version string if found, otherwise returns empty string.
func QueryVersion(bin string) string {
	cmd := fc.DefaultVMMCommandBuilder.
		WithBin(bin).
		WithArgs([]string{"--version"}).
		Build(context.Background())
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	// Extract semantic version from output (e.g. "Firecracker v0.24.0")
	re := regexp.MustCompile("[0-9]+\\.[0-9]+\\.[0-9]+")
	version := re.FindString(string(out))
	return version
}
