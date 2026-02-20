package utils

import (
	"context"
	"os/exec"
	"regexp"
	"time"
)

func QueryVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`[0-9]+\.[0-9]+\.[0-9]+`)
	version := re.FindString(string(out))
	return version
}
