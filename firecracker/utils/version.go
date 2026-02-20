//go:build !windows
// +build !windows

package utils

import (
	"context"
	"regexp"

	fc "github.com/firecracker-microvm/firecracker-go-sdk"
)

func QueryVersion(bin string) string {
	cmd := fc.DefaultVMMCommandBuilder.
		WithBin(bin).
		WithArgs([]string{"--version"}).
		Build(context.Background())
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile("[0-9]+\\.[0-9]+\\.[0-9]+")
	version := re.FindString(string(out))
	return version
}
