package main

import "testing"

func TestParseCLIArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    cliOptions
		wantErr bool
	}{
		{
			name: "no args",
			args: nil,
			want: cliOptions{},
		},
		{
			name: "claude short",
			args: []string{"-c"},
			want: cliOptions{launchClaude: true},
		},
		{
			name: "claude long with path",
			args: []string{"--claude", "."},
			want: cliOptions{launchClaude: true, pathArg: "."},
		},
		{
			name: "help",
			args: []string{"--help"},
			want: cliOptions{showHelp: true},
		},
		{
			name: "version",
			args: []string{"-v"},
			want: cliOptions{showVersion: true},
		},
		{
			name: "path only",
			args: []string{"/tmp"},
			want: cliOptions{pathArg: "/tmp"},
		},
		{
			name: "double dash path",
			args: []string{"--", "-file.txt"},
			want: cliOptions{pathArg: "-file.txt"},
		},
		{
			name:    "unknown option",
			args:    []string{"--nope"},
			wantErr: true,
		},
		{
			name:    "multiple paths",
			args:    []string{"a", "b"},
			wantErr: true,
		},
		{
			name:    "multiple after double dash",
			args:    []string{"--", "a", "b"},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseCLIArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected result: got %+v want %+v", got, tc.want)
			}
		})
	}
}

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
		name         string
		enablePaste  string
		disablePaste string
		want         bool
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
			name:        "enable override true with extra env values",
			enablePaste: "1",
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("EMED_ENABLE_PASTE", tc.enablePaste)
			t.Setenv("EMED_DISABLE_PASTE", tc.disablePaste)
			t.Setenv("SSH_CONNECTION", "127.0.0.1 123 127.0.0.1 22")
			t.Setenv("SSH_TTY", "/dev/ttys000")
			t.Setenv("TERM_PROGRAM", "iTerm.app")

			got := shouldEnableTerminalPaste()
			if got != tc.want {
				t.Fatalf("unexpected result: got %t want %t", got, tc.want)
			}
		})
	}
}
