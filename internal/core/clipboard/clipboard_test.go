package clipboard

import "testing"

func TestNormalizeLineEndings(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{name: "lf", in: "a\nb\n", out: "a\nb\n"},
		{name: "crlf", in: "a\r\nb\r\n", out: "a\nb\n"},
		{name: "cr", in: "a\rb\r", out: "a\nb\n"},
		{name: "mixed", in: "a\r\nb\rc\n", out: "a\nb\nc\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeLineEndings(tc.in)
			if got != tc.out {
				t.Fatalf("unexpected normalized text: got %q want %q", got, tc.out)
			}
		})
	}
}
