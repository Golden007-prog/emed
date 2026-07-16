package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStartersWhenFileMissing(t *testing.T) {
	store := NewStore(t.TempDir())

	starters, err := store.LoadStarters()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(starters) != 0 {
		t.Fatalf("expected empty list, got %d starters", len(starters))
	}
}

func TestSaveAndLoadStarters(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	saved := []Starter{
		{Name: "api server", Command: "pnpm dev", Unique: true},
		{Name: "tests", Command: "pnpm test --watch"},
	}
	if err := store.SaveStarters(saved); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := store.LoadStarters()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 starters, got %d", len(loaded))
	}
	if loaded[0] != saved[0] || loaded[1] != saved[1] {
		t.Fatalf("loaded starters differ: %+v", loaded)
	}

	if _, err := os.Stat(filepath.Join(root, ".emed", "starters.json")); err != nil {
		t.Fatalf("starters file not found at expected location: %v", err)
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	store := NewStore(t.TempDir())

	saved := Session{
		OpenFiles:  []string{"internal/ui/layout.go", "README.md"},
		ActiveFile: "README.md",
	}
	if err := store.SaveSession(saved); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded, err := store.LoadSession()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if loaded.ActiveFile != "README.md" || len(loaded.OpenFiles) != 2 {
		t.Fatalf("session round trip failed: %+v", loaded)
	}
}

func TestLoadSessionWhenFileMissing(t *testing.T) {
	store := NewStore(t.TempDir())

	session, err := store.LoadSession()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(session.OpenFiles) != 0 || session.ActiveFile != "" {
		t.Fatalf("expected empty session, got %+v", session)
	}
}

func TestLoadStartersWithCorruptFile(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	if err := os.MkdirAll(filepath.Join(root, ".emed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".emed", "starters.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := store.LoadStarters(); err == nil {
		t.Fatal("expected error for corrupt starters file")
	}
}

func TestOnDirCreatedFiresOnceOnFirstWrite(t *testing.T) {
	store := NewStore(t.TempDir())

	calls := 0
	store.SetOnDirCreated(func() { calls++ })

	if err := store.SaveSession(Session{OpenFiles: []string{"a.txt"}}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 callback call after first write, got %d", calls)
	}

	if err := store.SaveStarters([]Starter{{Name: "x", Command: "ls"}}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected callback to not fire again, got %d calls", calls)
	}
}

func TestValidateStarter(t *testing.T) {
	existing := []Starter{{Name: "api", Command: "pnpm dev"}}

	cases := []struct {
		name      string
		candidate Starter
		wantErr   bool
	}{
		{"valid", Starter{Name: "tests", Command: "go test ./..."}, false},
		{"empty name", Starter{Name: "  ", Command: "ls"}, true},
		{"empty command", Starter{Name: "x", Command: " "}, true},
		{"duplicated name", Starter{Name: "api", Command: "ls"}, true},
		{"duplicated name other case", Starter{Name: "API", Command: "ls"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateStarter(existing, tc.candidate)
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
