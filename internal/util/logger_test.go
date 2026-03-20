package util

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanOldLogs_RemovesOldFiles(t *testing.T) {
	dir := t.TempDir()

	// Create files: 3 old (>7 days) and 2 recent
	oldTime := time.Now().Add(-8 * 24 * time.Hour)
	recentTime := time.Now().Add(-1 * time.Hour)

	oldFiles := []string{
		"goanime_2026-03-01_10-00-00.log",
		"goanime_2026-03-02_10-00-00.log",
		"goanime_2026-03-03_10-00-00.log",
	}
	recentFiles := []string{
		"goanime_2026-03-19_10-00-00.log",
		"goanime_2026-03-20_10-00-00.log",
	}

	for _, name := range oldFiles {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("old log"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range recentFiles {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("recent log"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, recentTime, recentTime); err != nil {
			t.Fatal(err)
		}
	}

	cleanOldLogs(dir)

	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("expected 2 recent files remaining, got %d", len(entries))
	}

	for _, e := range entries {
		found := false
		for _, r := range recentFiles {
			if e.Name() == r {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected file remaining: %s", e.Name())
		}
	}
}

func TestCleanOldLogs_RespectsMaxFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 25 recent files (exceeds logMaxFiles=20)
	baseTime := time.Now().Add(-1 * time.Hour)

	for i := range 25 {
		name := filepath.Join(dir, "goanime_2026-03-20_10-00-"+padInt(i)+".log")
		if err := os.WriteFile(name, []byte("log"), 0o600); err != nil {
			t.Fatal(err)
		}
		// Each file 1 minute apart so they have different modtimes
		ft := baseTime.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(name, ft, ft); err != nil {
			t.Fatal(err)
		}
	}

	cleanOldLogs(dir)

	entries, _ := os.ReadDir(dir)
	if len(entries) != logMaxFiles {
		t.Errorf("expected %d files after cleanup, got %d", logMaxFiles, len(entries))
	}
}

func TestCleanOldLogs_IgnoresNonLogFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a non-log file and a non-goanime log file
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("keep"), 0o600)
	os.WriteFile(filepath.Join(dir, "other_app.log"), []byte("keep"), 0o600)

	oldTime := time.Now().Add(-30 * 24 * time.Hour)
	p := filepath.Join(dir, "goanime_2026-02-01_10-00-00.log")
	os.WriteFile(p, []byte("old"), 0o600)
	os.Chtimes(p, oldTime, oldTime)

	cleanOldLogs(dir)

	entries, _ := os.ReadDir(dir)
	// readme.txt and other_app.log should remain, goanime_ old file removed
	if len(entries) != 2 {
		t.Errorf("expected 2 non-goanime files remaining, got %d", len(entries))
	}
}

func TestCleanOldLogs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// Should not panic on empty directory
	cleanOldLogs(dir)
}

func TestCleanOldLogs_NonexistentDir(t *testing.T) {
	// Should not panic on nonexistent directory
	cleanOldLogs("/nonexistent/path/that/does/not/exist")
}

func padInt(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	s := ""
	s += string(rune('0' + n/10))
	s += string(rune('0' + n%10))
	return s
}
