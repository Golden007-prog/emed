package pathutil

import (
	"path/filepath"
	"testing"
)

func TestIsWithin(t *testing.T) {
	base := filepath.Join(string(filepath.Separator), "tmp", "emed-root")

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{
			name:   "same path",
			target: base,
			want:   true,
		},
		{
			name:   "child path",
			target: filepath.Join(base, "dir", "file.txt"),
			want:   true,
		},
		{
			name:   "sibling path",
			target: filepath.Join(string(filepath.Separator), "tmp", "emed-root-other", "file.txt"),
			want:   false,
		},
		{
			name:   "parent path",
			target: filepath.Join(string(filepath.Separator), "tmp", "file.txt"),
			want:   false,
		},
		{
			name:   "empty target",
			target: "",
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IsWithin(base, tc.target)
			if got != tc.want {
				t.Fatalf("unexpected result: got %t want %t", got, tc.want)
			}
		})
	}
}
