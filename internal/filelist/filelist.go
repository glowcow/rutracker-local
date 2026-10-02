// Package filelist encodes a torrent's file listing for storage in
// torrent_files.files: a compact JSON array of [path, size] pairs, deflated.
// JSON is what the API hands to the browser, so the stored bytes decompress
// straight into the response with no re-marshalling.
package filelist

import (
	"bytes"
	"compress/flate"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

// File is one entry of the listing. Path is slash-joined and relative to the
// torrent root; a single-file torrent has one entry with a bare name.
type File struct {
	Path string
	Size int64
}

// MaxDecompressed caps how much a blob may inflate to. A torrent with 33k
// files (the dump's worst case) lands near 2.5 MB, so 64 MB is far above any
// legitimate listing while still bounding a corrupt or hostile row.
const MaxDecompressed = 64 << 20

// Encode renders files as JSON and deflates the result.
func Encode(files []File) ([]byte, error) {
	var out bytes.Buffer
	zw, err := flate.NewWriter(&out, flate.DefaultCompression)
	if err != nil {
		return nil, fmt.Errorf("flate writer: %w", err)
	}
	if _, err := zw.Write(EncodeJSON(files)); err != nil {
		return nil, fmt.Errorf("compress file list: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("flush file list: %w", err)
	}
	return out.Bytes(), nil
}

// EncodeJSON marshals files as [["path",size],…] by hand: encoding/json's
// reflection path would run per entry, and the dump carries ~150M of them.
func EncodeJSON(files []File) []byte {
	// ~48 B/entry covers the median name plus punctuation; longer paths just
	// grow the buffer once or twice more.
	buf := make([]byte, 0, 2+len(files)*48)
	buf = append(buf, '[')
	for i, f := range files {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '[')
		buf = appendJSONString(buf, f.Path)
		buf = append(buf, ',')
		buf = appendInt(buf, f.Size)
		buf = append(buf, ']')
	}
	return append(buf, ']')
}

// DecodeJSON inflates a stored blob back to its JSON bytes, which the API
// embeds verbatim in its response.
func DecodeJSON(blob []byte) ([]byte, error) {
	zr := flate.NewReader(bytes.NewReader(blob))
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, MaxDecompressed+1))
	if err != nil {
		return nil, fmt.Errorf("decompress file list: %w", err)
	}
	if len(out) > MaxDecompressed {
		return nil, fmt.Errorf("file list exceeds %d bytes", MaxDecompressed)
	}
	return out, nil
}

// Decode inflates and parses a blob. Only callers that must rewrite the
// listing (truncating an oversized one) pay this cost — the plain read path
// uses DecodeJSON.
func Decode(blob []byte) ([]File, error) {
	raw, err := DecodeJSON(blob)
	if err != nil {
		return nil, err
	}
	var tuples [][]json.RawMessage
	if err := json.Unmarshal(raw, &tuples); err != nil {
		return nil, fmt.Errorf("parse file list: %w", err)
	}
	files := make([]File, 0, len(tuples))
	for _, t := range tuples {
		if len(t) != 2 {
			return nil, fmt.Errorf("file entry has %d fields, want 2", len(t))
		}
		var f File
		if err := json.Unmarshal(t[0], &f.Path); err != nil {
			return nil, fmt.Errorf("parse file path: %w", err)
		}
		if err := json.Unmarshal(t[1], &f.Size); err != nil {
			return nil, fmt.Errorf("parse file size: %w", err)
		}
		files = append(files, f)
	}
	return files, nil
}

const hexDigits = "0123456789abcdef"

// appendJSONString writes s as a JSON string literal. Valid UTF-8 above the
// control range passes through unescaped (the dump is full of Cyrillic —
// \u-escaping it would inflate every name); invalid bytes become U+FFFD so the
// output is always parseable JSON.
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			i++
			switch c {
			case '"', '\\':
				buf = append(buf, '\\', c)
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			default:
				if c < 0x20 {
					buf = append(buf, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF])
				} else {
					buf = append(buf, c)
				}
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			buf = append(buf, "�"...)
			i++
			continue
		}
		buf = append(buf, s[i:i+size]...)
		i += size
	}
	return append(buf, '"')
}

func appendInt(buf []byte, n int64) []byte {
	if n == 0 {
		return append(buf, '0')
	}
	if n < 0 {
		buf = append(buf, '-')
		n = -n
	}
	var tmp [20]byte
	p := len(tmp)
	for n > 0 {
		p--
		tmp[p] = byte('0' + n%10)
		n /= 10
	}
	return append(buf, tmp[p:]...)
}
