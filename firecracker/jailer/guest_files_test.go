package jailer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"
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
			must.EqOp(t, tt.want, got)
		})
	}
}

func TestLinkGuestFiles(t *testing.T) {
	tmp := t.TempDir()
	chrootRoot := filepath.Join(tmp, "chroot")
	srcDir := filepath.Join(tmp, "images")
	must.NoError(t, os.MkdirAll(srcDir, 0755))

	kernelPath := filepath.Join(srcDir, "vmlinux")
	drivePath := filepath.Join(srcDir, "rootfs.ext4")
	for _, p := range []string{kernelPath, drivePath} {
		must.NoError(t, os.WriteFile(p, []byte("data"), 0644))
	}

	// First link
	err := LinkGuestFiles(chrootRoot, kernelPath, "", []string{drivePath})
	must.NoError(t, err)

	// Verify files exist in chroot
	for _, name := range []string{"vmlinux", "rootfs.ext4"} {
		_, err := os.Stat(filepath.Join(chrootRoot, name))
		must.NoError(t, err, must.Sprintf("expected %s in chroot", name))
	}

	// Idempotent — second call should succeed with same files
	err = LinkGuestFiles(chrootRoot, kernelPath, "", []string{drivePath})
	must.NoError(t, err)
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
	must.Error(t, err)
}

func TestValidateAndResolvePath_Disallowed(t *testing.T) {
	_, err := ValidateAndResolvePath("/etc/passwd", "kernel", "/alloc", nil)
	must.Error(t, err)
}

func TestValidateAndResolvePath_Empty(t *testing.T) {
	path, err := ValidateAndResolvePath("", "kernel", "/alloc", nil)
	must.NoError(t, err)
	must.EqOp(t, "", path)
}

func TestPrepareGuestFiles(t *testing.T) {
	tmp := t.TempDir()
	chrootRoot := filepath.Join(tmp, "chroot")
	srcDir := filepath.Join(tmp, "images")
	must.NoError(t, os.MkdirAll(srcDir, 0755))

	kernelPath := filepath.Join(srcDir, "vmlinux")
	drivePath := filepath.Join(srcDir, "rootfs.ext4")
	for _, p := range []string{kernelPath, drivePath} {
		must.NoError(t, os.WriteFile(p, []byte("data"), 0644))
	}

	resKernel, resInitrd, resDrives, err := PrepareGuestFiles(
		chrootRoot, kernelPath, "", []string{drivePath},
		tmp, []string{srcDir},
	)
	must.NoError(t, err)
	must.EqOp(t, kernelPath, resKernel)
	must.EqOp(t, "", resInitrd)
	must.SliceLen(t, 1, resDrives)
	must.EqOp(t, drivePath, resDrives[0])

	// Verify files were linked into chroot.
	for _, name := range []string{"vmlinux", "rootfs.ext4"} {
		_, err := os.Stat(filepath.Join(chrootRoot, name))
		must.NoError(t, err, must.Sprintf("expected %s in chroot", name))
	}
}

func TestPrepareGuestFiles_DisallowedPath(t *testing.T) {
	tmp := t.TempDir()
	_, _, _, err := PrepareGuestFiles(
		filepath.Join(tmp, "chroot"), "/etc/passwd", "", nil,
		tmp, nil,
	)
	must.Error(t, err)
}
