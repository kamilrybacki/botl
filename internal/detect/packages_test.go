package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsDir(t *testing.T) {
	// Existing directory
	tmpDir := t.TempDir()
	if !isDir(tmpDir) {
		t.Errorf("isDir(%q) = false, want true", tmpDir)
	}

	// Non-existent path
	if isDir(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("isDir(nonexistent) = true, want false")
	}

	// File (not directory)
	tmpFile := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(tmpFile, []byte("test"), 0644)
	if isDir(tmpFile) {
		t.Error("isDir(file) = true, want false")
	}
}

func TestDetectGo_WithGOPATH(t *testing.T) {
	tmpDir := t.TempDir()
	modCache := filepath.Join(tmpDir, "pkg", "mod")
	os.MkdirAll(modCache, 0755)

	t.Setenv("GOPATH", tmpDir)

	mounts := detectGo()
	if len(mounts) != 1 {
		t.Fatalf("detectGo() returned %d mounts, want 1", len(mounts))
	}
	if mounts[0].Source != modCache {
		t.Errorf("Source = %q, want %q", mounts[0].Source, modCache)
	}
	if mounts[0].Target != modCache {
		t.Errorf("Target = %q, want %q", mounts[0].Target, modCache)
	}
}

func TestDetectGo_NoModCache(t *testing.T) {
	t.Setenv("GOPATH", "/nonexistent/gopath")
	mounts := detectGo()
	if len(mounts) != 0 {
		t.Errorf("detectGo() returned %d mounts for nonexistent GOPATH, want 0", len(mounts))
	}
}

func TestDetectRust_WithCargoRegistry(t *testing.T) {
	tmpHome := t.TempDir()
	registry := filepath.Join(tmpHome, ".cargo", "registry")
	os.MkdirAll(registry, 0755)

	t.Setenv("HOME", tmpHome)

	mounts := detectRust()
	if len(mounts) != 1 {
		t.Fatalf("detectRust() returned %d mounts, want 1", len(mounts))
	}
	if mounts[0].Source != registry {
		t.Errorf("Source = %q, want %q", mounts[0].Source, registry)
	}
}

func TestDetectRust_NoCargoDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mounts := detectRust()
	if len(mounts) != 0 {
		t.Errorf("detectRust() returned %d mounts for empty home, want 0", len(mounts))
	}
}

func TestHostPackages_ReturnsSlice(t *testing.T) {
	// HostPackages should never panic, even if all detectors find nothing
	mounts := HostPackages()
	if mounts == nil {
		// nil is acceptable (no packages found), but it should not panic
		return
	}
	// Each mount should have non-empty Source and Target
	for i, m := range mounts {
		if m.Source == "" {
			t.Errorf("mount[%d].Source is empty", i)
		}
		if m.Target == "" {
			t.Errorf("mount[%d].Target is empty", i)
		}
	}
}
