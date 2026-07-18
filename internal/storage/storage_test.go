package storage

import "testing"

func TestSafePath(t *testing.T) {
	for _, input := range []string{"../etc", "foo/../../bar", "/etc", "etc/passwd", "root", "var/run/docker.sock"} {
		if _, err := SafePath(input); err == nil {
			t.Fatalf("expected %q to be rejected", input)
		}
	}
	for input, want := range map[string]string{"": ".", "uploads/./avatars": "uploads/avatars", "file.txt": "file.txt"} {
		got, err := SafePath(input)
		if err != nil || got != want {
			t.Fatalf("SafePath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}
