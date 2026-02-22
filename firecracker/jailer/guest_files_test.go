package jailer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsAllowedImagePath(t *testing.T) {
	tests := []struct {
		name         string
		allowedPaths []string
		allocDir     string
		imagePath    string
		want         bool
	}{
		{
			name:      "relative path within alloc",
			allocDir:  "/alloc/data",
			imagePath: "kernel.bin",
			want:      true,
		},
		{
			name:      "absolute path in alloc",
			allocDir:  "/alloc/data",
			imagePath: "/alloc/data/images/kernel.bin",
			want:      true,
		},
		{
			name:         "absolute path in allowed dir",
			allowedPaths: []string{"/opt/images"},
			allocDir:     "/alloc/data",
			imagePath:    "/opt/images/kernel.bin",
			want:         true,
		},
		{
			name:      "path escaping alloc via ..",
			allocDir:  "/alloc/data",
			imagePath: "/alloc/data/../../etc/passwd",
			want:      false,
		},
		{
			name:         "path not in any allowed dir",
			allowedPaths: []string{"/opt/images"},
			allocDir:     "/alloc/data",
			imagePath:    "/etc/passwd",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAllowedImagePath(tt.allowedPaths, tt.allocDir, tt.imagePath)
			if got != tt.want {
				t.Errorf("isAllowedImagePath = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLinkGuestFiles(t *testing.T) {
	tmp := t.TempDir()
	chrootRoot := filepath.Join(tmp, "chroot")
	srcDir := filepath.Join(tmp, "images")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}

	kernelPath := filepath.Join(srcDir, "vmlinux")
	drivePath := filepath.Join(srcDir, "rootfs.ext4")
	for _, p := range []string{kernelPath, drivePath} {
		if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// First link
	err := LinkGuestFiles(chrootRoot, kernelPath, "", []string{drivePath})
	if err != nil {
		t.Fatalf("LinkGuestFiles: %v", err)
	}

	// Verify files exist in chroot
	for _, name := range []string{"vmlinux", "rootfs.ext4"} {
		if _, err := os.Stat(filepath.Join(chrootRoot, name)); err != nil {
			t.Errorf("expected %s in chroot: %v", name, err)
		}
	}

	// Idempotent — second call should succeed with same files
	err = LinkGuestFiles(chrootRoot, kernelPath, "", []string{drivePath})
	if err != nil {
		t.Fatalf("idempotent LinkGuestFiles: %v", err)
	}
}

func TestLinkGuestFiles_DuplicateBasename(t *testing.T) {
	tmp := t.TempDir()
	dir1 := filepath.Join(tmp, "a")
	dir2 := filepath.Join(tmp, "b")
	for _, d := range []string{dir1, dir2} {
		os.MkdirAll(d, 0755)
		os.WriteFile(filepath.Join(d, "image.bin"), []byte("x"), 0644)
	}

	err := LinkGuestFiles(filepath.Join(tmp, "chroot"),
		filepath.Join(dir1, "image.bin"), "",
		[]string{filepath.Join(dir2, "image.bin")})
	if err == nil {
		t.Fatal("expected error for duplicate basename from different sources")
	}
}

func TestValidateAndResolvePath_Disallowed(t *testing.T) {
	_, err := ValidateAndResolvePath("/etc/passwd", "kernel", "/alloc", nil)
	if err == nil {
		t.Fatal("expected error for disallowed path")
	}
}

func TestValidateAndResolvePath_Empty(t *testing.T) {
	path, err := ValidateAndResolvePath("", "kernel", "/alloc", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "" {
		t.Errorf("expected empty, got %q", path)
	}
}
