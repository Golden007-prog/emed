package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// UserConfig holds user level preferences, loaded from
// ~/.emed/config.json. A missing or invalid file means defaults.
type UserConfig struct {
	// UseTerminalBackground makes emed use the terminal own default
	// background instead of fixed colors, so semi-transparent terminal
	// setups keep their effect. Enabled by default, designed for dark
	// terminal themes; set it to false for fixed opaque colors.
	UseTerminalBackground bool `json:"useTerminalBackground"`
}

func defaultUserConfig() UserConfig {
	return UserConfig{UseTerminalBackground: true}
}

// LoadUserConfig reads the user config file, returning defaults when
// the file does not exist or can not be parsed.
func LoadUserConfig() UserConfig {
	cfg := defaultUserConfig()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}

	data, err := os.ReadFile(filepath.Join(homeDir, ".emed", "config.json"))
	if err != nil {
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultUserConfig()
	}
	return cfg
}
