package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUserConfigMissingFileUsesDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg := LoadUserConfig()
	if !cfg.UseTerminalBackground {
		t.Error("expected terminal background enabled by default")
	}
}

func TestLoadUserConfigReadsValues(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".emed"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"useTerminalBackground": false}` + "\n")
	if err := os.WriteFile(filepath.Join(home, ".emed", "config.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadUserConfig()
	if cfg.UseTerminalBackground {
		t.Error("expected useTerminalBackground false from config file")
	}
}

func TestLoadUserConfigCorruptFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := os.MkdirAll(filepath.Join(home, ".emed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".emed", "config.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := LoadUserConfig()
	if !cfg.UseTerminalBackground {
		t.Error("expected defaults for corrupt config file")
	}
}
