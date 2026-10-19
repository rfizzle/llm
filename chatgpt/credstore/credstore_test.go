package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A store with no directory answers every operation the same way, so the
// guard does not depend on Open having refused it first.
func TestFileStore_NoDirectoryIsRefusedEverywhere(t *testing.T) {
	var s FileStore
	if _, err := s.LoadRecord(); !errors.Is(err, errNoDir) {
		t.Errorf("LoadRecord = %v", err)
	}
	if _, err := s.LoadHostID(); !errors.Is(err, errNoDir) {
		t.Errorf("LoadHostID = %v", err)
	}
	if err := s.SaveHostID("urn:uuid:x"); !errors.Is(err, errNoDir) {
		t.Errorf("SaveHostID = %v", err)
	}
	if f, ok, err := s.TryLock(); !errors.Is(err, errNoDir) || f != nil || ok {
		t.Errorf("TryLock = %v, %v, %v", f != nil, ok, err)
	}
}

// A write that fails part-way leaves the old record whole and no temporary
// file behind, whichever step failed.
func TestReplaceFile_AFailureLeavesTheOldFileWhole(t *testing.T) {
	failed := errors.New("disk said no")
	for name, fail := range map[string]func(){
		"sync":   func() { syncFile = func(*os.File) error { return failed } },
		"rename": func() { renameFile = func(string, string) error { return failed } },
	} {
		t.Run(name, func(t *testing.T) {
			savedSync, savedRename := syncFile, renameFile
			t.Cleanup(func() { syncFile, renameFile = savedSync, savedRename })
			dir := t.TempDir()
			path := filepath.Join(dir, "credentials.json")
			if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			fail()
			if err := replaceFile(path, "new\n", 0o600); !errors.Is(err, failed) {
				t.Fatalf("replaceFile = %v, want the disk's error", err)
			}
			if got, _ := os.ReadFile(path); string(got) != "old\n" {
				t.Errorf("the record = %q, want the old one whole", got)
			}
			entries, _ := os.ReadDir(dir)
			if len(entries) != 1 {
				t.Errorf("a temporary file was left behind: %v", entries)
			}
		})
	}
}

// The directory is synced after the rename, and a directory that will not
// sync is not a failed write.
func TestReplaceFile_SyncsTheDirectoryAfterTheRename(t *testing.T) {
	savedRename, savedDir := renameFile, syncDir
	t.Cleanup(func() { renameFile, syncDir = savedRename, savedDir })
	var order []string
	renameFile = func(from, to string) error { order = append(order, "rename"); return savedRename(from, to) }
	syncDir = func(string) error { order = append(order, "sync dir"); return errors.New("not supported") }
	path := filepath.Join(t.TempDir(), "host-id")
	if err := replaceFile(path, "a\n", 0o600); err != nil {
		t.Fatalf("a directory that will not sync failed the write: %v", err)
	}
	if got := strings.Join(order, ","); got != "rename,sync dir" {
		t.Errorf("order = %s", got)
	}
}
