// Package filelist stores a torrent's files as a deflated JSON array of
// [path, size], the JSON the API sends, so it inflates straight into replies.
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

// MaxDecompressed bounds a blob's inflated size: the largest listing, 33k
// files, is about 2.5 MB.
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
	for i, file := range files {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, '[')
		buf = appendJSONString(buf, file.Path)
		buf = append(buf, ',')
		buf = appendInt(buf, file.Size)
		buf = append(buf, ']')
	}
	return append(buf, ']')
}

// DecodeJSON inflates a stored blob back to its JSON bytes, which the API
// embeds verbatim in its response.
func DecodeJSON(blob []byte) ([]byte, error) {
	zr := flate.NewReader(bytes.NewReader(blob))
	defer func() { _ = zr.Close() }()
	out, err := io.ReadAll(io.LimitReader(zr, MaxDecompressed+1))
	if err != nil {
		return nil, fmt.Errorf("decompress file list: %w", err)
	}
	if len(out) > MaxDecompressed {
		return nil, fmt.Errorf("file list exceeds %d bytes", MaxDecompressed)
	}
	return out, nil
}

// Decode inflates and parses a blob, for a listing that must be cut; reads
// use DecodeJSON.
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
	for _, tuple := range tuples {
		if len(tuple) != 2 {
			return nil, fmt.Errorf("file entry has %d fields, want 2", len(tuple))
		}
		var file File
		if err := json.Unmarshal(tuple[0], &file.Path); err != nil {
			return nil, fmt.Errorf("parse file path: %w", err)
		}
		if err := json.Unmarshal(tuple[1], &file.Size); err != nil {
			return nil, fmt.Errorf("parse file size: %w", err)
		}
		files = append(files, file)
	}
	return files, nil
}

const hexDigits = "0123456789abcdef"

// appendJSONString writes str as JSON, Cyrillic unescaped to keep it small
// and invalid bytes as U+FFFD to keep it valid.
func appendJSONString(buf []byte, str string) []byte {
	buf = append(buf, '"')
	for i := 0; i < len(str); {
		char := str[i]
		if char < utf8.RuneSelf {
			i++
			switch char {
			case '"', '\\':
				buf = append(buf, '\\', char)
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			default:
				if char < 0x20 {
					buf = append(buf, '\\', 'u', '0', '0', hexDigits[char>>4], hexDigits[char&0xF])
				} else {
					buf = append(buf, char)
				}
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(str[i:])
		if r == utf8.RuneError && size == 1 {
			buf = append(buf, "�"...)
			i++
			continue
		}
		buf = append(buf, str[i:i+size]...)
		i += size
	}
	return append(buf, '"')
}

func appendInt(buf []byte, num int64) []byte {
	if num == 0 {
		return append(buf, '0')
	}
	if num < 0 {
		buf = append(buf, '-')
		num = -num
	}
	var tmp [20]byte
	pos := len(tmp)
	for num > 0 {
		pos--
		tmp[pos] = byte('0' + num%10)
		num /= 10
	}
	return append(buf, tmp[pos:]...)
}
