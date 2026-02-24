package main

import "testing"

func TestParseBoolEnv(t *testing.T) {
	t.Setenv("EMED_TEST_BOOL", "")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); ok || value {
		t.Fatalf("expected empty value to be unset, got value=%t ok=%t", value, ok)
	}

	t.Setenv("EMED_TEST_BOOL", "true")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); !ok || !value {
		t.Fatalf("expected true to parse as true, got value=%t ok=%t", value, ok)
	}

	t.Setenv("EMED_TEST_BOOL", "off")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); !ok || value {
		t.Fatalf("expected off to parse as false, got value=%t ok=%t", value, ok)
	}

	t.Setenv("EMED_TEST_BOOL", "maybe")
	if value, ok := parseBoolEnv("EMED_TEST_BOOL"); ok || value {
		t.Fatalf("expected invalid value to be unset, got value=%t ok=%t", value, ok)
	}
}

func TestShouldEnableTerminalPaste(t *testing.T) {
	tests := []struct {
		name          string
		enablePaste   string
		disablePaste  string
		sshConnection string
		sshTTY        string
		termProgram   string
		want          bool
	}{
		{
			name: "default",
			want: true,
		},
		{
			name:        "enable override false",
			enablePaste: "0",
			want:        false,
		},
		{
			name:        "enable override true",
			enablePaste: "1",
			want:        true,
		},
		{
			name:         "disable override true",
			disablePaste: "1",
			want:         false,
		},
		{
			name:         "disable override false",
			disablePaste: "0",
			want:         true,
		},
		{
			name:          "enable override with ssh env",
			enablePaste:   "1",
			sshConnection: "127.0.0.1 123 127.0.0.1 22",
			want:          true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EMED_ENABLE_PASTE", tc.enablePaste)
			t.Setenv("EMED_DISABLE_PASTE", tc.disablePaste)
			t.Setenv("SSH_CONNECTION", tc.sshConnection)
			t.Setenv("SSH_TTY", tc.sshTTY)
			t.Setenv("TERM_PROGRAM", tc.termProgram)

			got := shouldEnableTerminalPaste()
			if got != tc.want {
				t.Fatalf("unexpected result: got %t want %t", got, tc.want)
			}
		})
	}
}
