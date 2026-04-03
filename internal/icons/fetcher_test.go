package icons

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestIsValidIconID verifies the allowlist for safe icon identifiers.
func TestIsValidIconID(t *testing.T) {
	valid := []string{
		"wireguard",
		"adguard-home",
		"my_app",
		"App123",
		"a",
	}
	for _, id := range valid {
		if !isValidIconID(id) {
			t.Errorf("isValidIconID(%q) = false; want true", id)
		}
	}

	invalid := []string{
		"",
		"../etc/passwd",
		"../../secret",
		"foo/bar",
		"foo\\bar",
		"foo.bar",
		"foo bar",
		"foo\x00bar",
	}
	for _, id := range invalid {
		if isValidIconID(id) {
			t.Errorf("isValidIconID(%q) = true; want false", id)
		}
	}
}

// TestSafeIconPath verifies that safeIconPath rejects traversal attempts.
func TestSafeIconPath(t *testing.T) {
	dir := t.TempDir()
	f := &Fetcher{iconsDir: dir}

	good, err := f.safeIconPath("wireguard")
	if err != nil {
		t.Fatalf("safeIconPath(wireguard): unexpected error: %v", err)
	}
	want := filepath.Join(dir, "wireguard.svg")
	if good != want {
		t.Errorf("safeIconPath(wireguard) = %q; want %q", good, want)
	}

	traversals := []string{
		"../secret",
		"../../etc/passwd",
	}
	for _, id := range traversals {
		_, err := f.safeIconPath(id)
		if err == nil {
			t.Errorf("safeIconPath(%q) should have returned error", id)
		}
	}
}

// TestServeIcon_TraversalRejected verifies that ServeIcon returns false for
// path traversal inputs without serving any file content.
func TestServeIcon_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	f := &Fetcher{iconsDir: dir, client: nil}

	attacks := []string{
		"../../../etc/passwd",
		"../../secret",
		"foo/bar",
		"foo\\bar",
	}
	for _, id := range attacks {
		w := httptest.NewRecorder()
		if f.ServeIcon(w, id) {
			t.Errorf("ServeIcon(%q) returned true; expected false (path traversal)", id)
		}
		// Body must be empty — no file content was served.
		if w.Body.Len() > 0 {
			t.Errorf("ServeIcon(%q) wrote body content; expected empty response", id)
		}
	}
}

// TestEnsureIcon_TraversalRejected verifies EnsureIcon is a no-op for invalid IDs.
func TestEnsureIcon_TraversalRejected(t *testing.T) {
	dir := t.TempDir()
	f := &Fetcher{iconsDir: dir, client: nil}

	// Plant a sentinel file that a traversal might try to reach.
	sentinel := filepath.Join(dir, "sentinel.txt")
	_ = os.WriteFile(sentinel, []byte("secret"), 0600)

	// These should silently no-op; they must not panic or access sentinel.
	f.EnsureIcon("../sentinel", "")
	f.EnsureIcon("../../etc/passwd", "")
}
