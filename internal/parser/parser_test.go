package parser

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/glowcow/rutracker-local/internal/filelist"
	"github.com/glowcow/rutracker-local/internal/store"
)

func TestParseFile_TinyFixture(t *testing.T) {
	var got []store.Torrent
	if err := ParseFile("testdata/tiny.xml", func(rec store.Torrent) error {
		got = append(got, rec)
		return nil
	}); err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if want := 3; len(got) != want {
		t.Fatalf("got %d records, want %d", len(got), want)
	}

	// Record 0 — checks title trim, forum-with-em-dash, BBCode passthrough.
	r0 := got[0]
	if r0.ID != 100 {
		t.Errorf("[0].ID = %d, want 100", r0.ID)
	}
	if r0.Title != "Test Album One" { // outer spaces trimmed
		t.Errorf("[0].Title = %q, want %q (trimmed)", r0.Title, "Test Album One")
	}
	if r0.ForumID != 42 {
		t.Errorf("[0].ForumID = %d, want 42", r0.ForumID)
	}
	if r0.ForumName != "Music — Rock" {
		t.Errorf("[0].ForumName = %q, want %q", r0.ForumName, "Music — Rock")
	}
	if r0.SizeBytes != 1024 {
		t.Errorf("[0].SizeBytes = %d, want 1024", r0.SizeBytes)
	}
	if r0.Hash != "AAAA1234567890ABCDEFABCDEFABCDEFABCDEF00" {
		t.Errorf("[0].Hash = %q, want AAAA...", r0.Hash)
	}
	if got, want := r0.RegisteredAt.UTC(), time.Unix(1577872800, 0).UTC(); !got.Equal(want) {
		t.Errorf("[0].RegisteredAt = %v, want %v", got, want)
	}
	if r0.Content == "" {
		t.Error("[0].Content was empty; expected BBCode passthrough")
	}
	// File tree: files of a level precede its subdirectories, paths are
	// slash-joined from the root <dir>, XML entities are decoded.
	wantFiles := []filelist.File{
		{Path: `Test Album One/01 — Track & "One".flac`, Size: 1024},
		{Path: "Test Album One/Artwork/Front.jpg", Size: 4096},
	}
	if !reflect.DeepEqual(r0.Files, wantFiles) {
		t.Errorf("[0].Files = %+v, want %+v", r0.Files, wantFiles)
	}

	// Record 1 — large size (>int32) to verify int64 storage.
	r1 := got[1]
	if r1.SizeBytes != 2147483648 {
		t.Errorf("[1].SizeBytes = %d, want 2147483648 (2 GB)", r1.SizeBytes)
	}
	// Single-file torrent: a bare <file>, no wrapping directory.
	if want := []filelist.File{{Path: "Test Film Two.mkv", Size: 2147483648}}; !reflect.DeepEqual(r1.Files, want) {
		t.Errorf("[1].Files = %+v, want %+v", r1.Files, want)
	}

	// Record 2 — empty content + zero size, edge cases.
	r2 := got[2]
	if r2.Content != "" {
		t.Errorf("[2].Content = %q, want empty", r2.Content)
	}
	if r2.SizeBytes != 0 {
		t.Errorf("[2].SizeBytes = %d, want 0", r2.SizeBytes)
	}
	// No <dir>/<file> at all — the store skips such rows rather than writing
	// an empty listing.
	if len(r2.Files) != 0 {
		t.Errorf("[2].Files = %+v, want none", r2.Files)
	}
}

func TestResolveSource_LiteralPath(t *testing.T) {
	got, err := ResolveSource("/some/literal/path.xml.xz")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "/some/literal/path.xml.xz" {
		t.Errorf("got %q, want literal passthrough", got)
	}
}

func TestResolveSource_GlobPicksNewestMtime(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "rutracker-20240101.xml.xz")
	middle := filepath.Join(dir, "rutracker-20250425.xml.xz")
	newest := filepath.Join(dir, "rutracker-20260530.xml.xz")
	for _, p := range []string{older, middle, newest} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		path string
		mod  time.Time
	}{
		{older, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{middle, time.Date(2025, 4, 25, 0, 0, 0, 0, time.UTC)},
		{newest, time.Date(2026, 5, 30, 0, 0, 0, 0, time.UTC)},
	} {
		if err := os.Chtimes(tc.path, tc.mod, tc.mod); err != nil {
			t.Fatal(err)
		}
	}

	got, err := ResolveSource(filepath.Join(dir, "rutracker-*.xml.xz"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != newest {
		t.Errorf("got %q, want %q (newest by mtime)", got, newest)
	}
}

func TestResolveSource_NoMatch(t *testing.T) {
	dir := t.TempDir()
	_, err := ResolveSource(filepath.Join(dir, "nothing-*.xml.xz"))
	if err == nil {
		t.Fatal("expected error for empty glob, got nil")
	}
	if !strings.Contains(err.Error(), "no files match") {
		t.Errorf("err message lost context: %v", err)
	}
}

func TestParseFile_NonexistentPath(t *testing.T) {
	err := ParseFile("testdata/does-not-exist.xml", func(store.Torrent) error { return nil })
	if err == nil {
		t.Fatal("expected error opening missing file, got nil")
	}
	if _, ok := err.(*os.PathError); !ok {
		// xml errors get wrapped; opening errors are *PathError under our %w wrap.
		// Use errors.Is via os.IsNotExist for a robust check.
		if !os.IsNotExist(err) {
			t.Logf("err (not a PathError, not IsNotExist): %v", err)
		}
	}
}
