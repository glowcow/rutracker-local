// Package parser streams a RuTracker XML dump, plain, xz or gzip, and hands
// each torrent to a callback.
package parser

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/glowcow/rutracker-local/internal/filelist"
	"github.com/glowcow/rutracker-local/internal/store"
)

// ResolveSource returns path, or the newest file a glob matches (by mtime,
// which survives renames and partial downloads).
func ResolveSource(pattern string) (string, error) {
	if !strings.ContainsAny(pattern, "*?[") {
		return pattern, nil
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("glob %q: %w", pattern, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no files match glob %q", pattern)
	}
	var (
		newest  string
		newestT time.Time
	)
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			continue
		}
		if st.ModTime().After(newestT) {
			newest = m
			newestT = st.ModTime()
		}
	}
	if newest == "" {
		return "", fmt.Errorf("none of %d matches for %q were stat-able", len(matches), pattern)
	}
	return newest, nil
}

// xmlTorrent is a <torrent>; the infohash is the nested <torrent hash/>, and
// the misspelled registred_at is ignored for unixts.
type xmlTorrent struct {
	XMLName xml.Name `xml:"torrent"`
	ID      int64    `xml:"id,attr"`
	Unixts  int64    `xml:"unixts,attr"`
	Size    int64    `xml:"size,attr"`
	Title   string   `xml:"title"`
	Inner   struct {
		Hash      string `xml:"hash,attr"`
		TrackerID int    `xml:"tracker_id,attr"`
	} `xml:"torrent"`
	Forum struct {
		ID   int32  `xml:"id,attr"`
		Name string `xml:",chardata"`
	} `xml:"forum"`
	Content string `xml:"content"`
	// File listing: a <dir> tree for multi-file torrents, a bare <file> for
	// single-file ones. Both forms appear at this level.
	Dirs  []xmlDir  `xml:"dir"`
	Files []xmlFile `xml:"file"`
}

// xmlDir/xmlFile mirror the dump's file tree, which nests arbitrarily deep.
type xmlDir struct {
	Name  string    `xml:"name,attr"`
	Dirs  []xmlDir  `xml:"dir"`
	Files []xmlFile `xml:"file"`
}

type xmlFile struct {
	Name string `xml:"name,attr"`
	Size int64  `xml:"size,attr"`
}

// flattenFiles joins the tree into slash paths, files before directories:
// encoding/xml has already split the two apart.
func flattenFiles(prefix string, dirs []xmlDir, files []xmlFile, out []filelist.File) []filelist.File {
	for _, f := range files {
		out = append(out, filelist.File{Path: prefix + f.Name, Size: f.Size})
	}
	for _, d := range dirs {
		out = flattenFiles(prefix+d.Name+"/", d.Dirs, d.Files, out)
	}
	return out
}

// Emit is the callback invoked for each parsed record. Returning an error
// aborts parsing. Returning context.Canceled etc. propagates up.
type Emit func(store.Torrent) error

// Parse calls emit for every <torrent> of the raw XML in r; ParseFile
// decompresses.
func Parse(r io.Reader, emit Emit) error {
	dec := xml.NewDecoder(r)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("xml token: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "torrent" {
			continue
		}

		var x xmlTorrent
		if err := dec.DecodeElement(&x, &se); err != nil {
			return fmt.Errorf("decode torrent (offset %d): %w", dec.InputOffset(), err)
		}

		// DecodeElement consumed the nested <torrent hash/> with the outer.

		if err := emit(store.Torrent{
			ID:           x.ID,
			Title:        strings.TrimSpace(x.Title),
			ForumID:      x.Forum.ID,
			ForumName:    strings.TrimSpace(x.Forum.Name),
			SizeBytes:    x.Size,
			RegisteredAt: time.Unix(x.Unixts, 0).UTC(),
			Hash:         x.Inner.Hash,
			Content:      x.Content,
			Files:        flattenFiles("", x.Dirs, x.Files, nil),
		}); err != nil {
			return err
		}
	}
}

// ParseFile parses a .xml, a .xml.xz (through native xz, pure Go was too slow)
// or a .xml.gz, and closes them before it returns.
func ParseFile(path string, emit Emit) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	r, err := decompressorFor(path, f)
	if err != nil {
		return err
	}
	// Close reaps xz: a truncated dump shows only in its exit status.
	err = Parse(r, emit)
	if cerr := r.Close(); err == nil {
		err = cerr
	}
	return err
}

// DecompressorFor decompresses src by path's extension; Close must be called,
// for .xz it reaps xz and reports its exit status.
func DecompressorFor(path string, src io.Reader) (io.ReadCloser, error) {
	return decompressorFor(path, src)
}

func decompressorFor(path string, src io.Reader) (io.ReadCloser, error) {
	switch {
	case strings.HasSuffix(path, ".xz"):
		return newXZReader(src)
	case strings.HasSuffix(path, ".gz"):
		r, err := gzip.NewReader(src)
		if err != nil {
			return nil, fmt.Errorf("gzip reader for %s: %w", path, err)
		}
		return r, nil
	default:
		return io.NopCloser(src), nil
	}
}

// xzReader runs xz -dc on the caller's reader, so a counting reader still sees
// the compressed bytes; --threads helps a multi-block dump.
type xzReader struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr *bytes.Buffer
}

func newXZReader(src io.Reader) (io.ReadCloser, error) {
	cmd := exec.Command("xz", "-dc", "--threads=4")
	cmd.Stdin = src
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("xz stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("xz start: %w", err)
	}
	return &xzReader{cmd: cmd, stdout: stdout, stderr: &stderr}, nil
}

func (r *xzReader) Read(p []byte) (int, error) { return r.stdout.Read(p) }

func (r *xzReader) Close() error {
	_ = r.stdout.Close()
	if err := r.cmd.Wait(); err != nil {
		return fmt.Errorf("xz exit: %w (stderr=%q)", err, r.stderr.String())
	}
	return nil
}
