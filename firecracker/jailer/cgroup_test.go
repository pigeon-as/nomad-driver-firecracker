package jailer

import "testing"

func TestDetectCgroupVersion(t *testing.T) {
	v := DetectCgroupVersion()
	switch v {
	case "", "1", "2":
		// valid
	default:
		t.Errorf("DetectCgroupVersion() = %q, want \"\", \"1\", or \"2\"", v)
	}
}
