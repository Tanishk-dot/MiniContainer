package unionfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWhiteoutHelpers(t *testing.T) {
	if !IsWhiteout(".wh.file") {
		t.Fatal("IsWhiteout(.wh.file) = false, want true")
	}
	target, ok := WhiteoutTarget(".wh.file")
	if !ok || target != "file" {
		t.Fatalf("WhiteoutTarget() = (%q, %v), want (file, true)", target, ok)
	}
	if !IsOpaqueWhiteout(OpaqueWhiteout) {
		t.Fatal("IsOpaqueWhiteout() = false, want true")
	}
}

func TestCreateWhiteoutOnDisk(t *testing.T) {
	dir := t.TempDir()
	if err := CreateWhiteout(dir, "removed.txt"); err != nil {
		t.Fatalf("CreateWhiteout() error = %v", err)
	}

	path := filepath.Join(dir, WhiteoutPrefix+"removed.txt")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("whiteout file missing: %v", err)
	}
}
