package vpath

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"unicode/utf8"
)

// TestFilesystemCollisions probes the actual volume using exclusive file creates.
// Set M3_FS_PROBE=1 explicitly; CI verifies NTFS before enabling it on Windows.
func TestFilesystemCollisions(t *testing.T) {
	if os.Getenv("M3_FS_PROBE") != "1" {
		t.Skip("set M3_FS_PROBE=1 on the volume to measure")
	}
	golden := readGolden(t)
	for i, p := range golden.Pairs {
		dir := t.TempDir()
		a := filepath.Join(dir, p.A)
		b := filepath.Join(dir, p.B)
		// Golden inputs may contain directory components. The filesystem probe
		// compares single names only; ownership paths are covered by pure tests.
		if filepath.Base(a) != p.A || filepath.Base(b) != p.B {
			continue
		}
		f, e := os.OpenFile(a, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- committed single-component golden in a fresh temporary directory.
		if e != nil {
			t.Fatal(e)
		}
		if e = f.Close(); e != nil {
			t.Fatal(e)
		}
		f, e = os.OpenFile(b, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- committed single-component golden in a fresh temporary directory.
		collides := errors.Is(e, os.ErrExist)
		if e != nil && !collides {
			t.Fatal(e)
		}
		if e == nil {
			if e = f.Close(); e != nil {
				t.Fatal(e)
			}
		}
		t.Logf("pair=%d os=%s a=%q b=%q collision=%t key_equal=%t", i, runtime.GOOS, p.A, p.B, collides, pathKey(p.A) == pathKey(p.B))
		if collides && pathKey(p.A) != pathKey(p.B) {
			t.Errorf("filesystem collision missed: %q / %q", p.A, p.B)
		}
	}
}

// TestNTFSUpcase checks every BMP mapping in the actual on-disk $UpCase table.
// This is useful without a mount, but is not a Windows file-creation test.
func TestNTFSUpcase(t *testing.T) {
	p := os.Getenv("M3_NTFS_UPCASE")
	if p == "" {
		t.Skip("set M3_NTFS_UPCASE to an extracted 131072-byte table")
	}
	b, e := os.ReadFile(p) // #nosec G304 G703 -- explicit local test-only input.
	if e != nil {
		t.Fatal(e)
	}
	if len(b) != 65536*2 {
		t.Fatalf("unexpected $UpCase size: %d", len(b))
	}
	mappings := 0
	for n := 0; n < 65536; n++ {
		r := rune(n)
		u := rune(binary.LittleEndian.Uint16(b[n*2:]))
		if !utf8.ValidRune(r) || !utf8.ValidRune(u) {
			continue
		}
		if r == u {
			continue
		}
		mappings++
		if pathKey(string(r)) != pathKey(string(u)) {
			t.Errorf("missed NTFS mapping %U -> %U", r, u)
		}
	}
	t.Logf("checked all 65,536 entries (%d nonidentity scalar mappings)", mappings)
}
