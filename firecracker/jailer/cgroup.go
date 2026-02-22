package jailer

import "os"

// DetectCgroupVersion returns "1" or "2" matching jailer's --cgroup-version flag.
func DetectCgroupVersion() string {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return "2"
	}
	if _, err := os.Stat("/sys/fs/cgroup/cpu"); err == nil {
		return "1"
	}
	return ""
}
