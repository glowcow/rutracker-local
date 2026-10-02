package filelist

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	files := []File{
		{Path: "Album/01 — Track.flac", Size: 195885721},
		{Path: `Album/Scans/back "cover".jpg`, Size: 0},
		{Path: "Album/log\ttab.txt", Size: 9007199254740993}, // beyond float64 precision
	}
	blob, err := Encode(files)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(blob)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(got, files) {
		t.Errorf("round trip = %+v, want %+v", got, files)
	}
}

// The hand-rolled writer must agree with encoding/json byte for byte —
// the API ships its output straight to the browser.
func TestEncodeJSONMatchesStdlib(t *testing.T) {
	files := []File{
		{Path: `Кириллица/файл "один"\два.mkv`, Size: 42},
		{Path: "ctrl\x01\n\r\tchars", Size: -1},
		{Path: string([]byte{0xff, 'a'}), Size: 1}, // invalid UTF-8 → U+FFFD
	}
	want, err := json.Marshal([][2]any{
		{`Кириллица/файл "один"\два.mkv`, 42},
		{"ctrl\x01\n\r\tchars", -1},
		{"�a", 1},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(EncodeJSON(files)); got != string(want) {
		t.Errorf("EncodeJSON =\n%s\nwant\n%s", got, want)
	}
}

func TestEncodeEmpty(t *testing.T) {
	blob, err := Encode(nil)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	raw, err := DecodeJSON(blob)
	if err != nil {
		t.Fatalf("DecodeJSON: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("DecodeJSON = %q, want []", raw)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, err := DecodeJSON([]byte("not deflate at all")); err == nil {
		t.Error("DecodeJSON accepted garbage")
	}
}

// A blob that inflates past MaxDecompressed must fail rather than being read
// into memory whole.
func TestDecodeRejectsBomb(t *testing.T) {
	huge := []File{{Path: strings.Repeat("a", 1024), Size: 1}}
	for len(huge) < 70_000 { // ~70 MB of paths, compresses to a few KB
		huge = append(huge, huge...)
	}
	blob, err := Encode(huge)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := DecodeJSON(blob); err == nil {
		t.Error("DecodeJSON accepted an over-sized listing")
	}
}
