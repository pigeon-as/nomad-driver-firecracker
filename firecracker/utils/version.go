package utils

import (
	"context"
	"os/exec"
	"regexp"
	"time"
)

var versionRegex = regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)

func QueryVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	version := versionRegex.FindString(string(out))
	return version
}
