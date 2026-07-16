// Package project handles per project configuration and state, stored
// in the .emed directory at the project root.
package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	storeDirName     = ".emed"
	startersFileName = "starters.json"
	sessionFileName  = "session.json"
)

// Starter is a named terminal command configured for the project.
type Starter struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Unique  bool   `json:"unique"`
}

// Session holds the editor state that is restored when the project
// is opened again. Paths are relative to the project root.
type Session struct {
	OpenFiles  []string `json:"openFiles"`
	ActiveFile string   `json:"activeFile,omitempty"`
}

// Store reads and writes project scoped files under <root>/.emed.
type Store struct {
	rootPath     string
	mu           sync.Mutex
	onDirCreated func()
}

// NewStore creates a store bound to the given project root.
func NewStore(rootPath string) *Store {
	return &Store{rootPath: rootPath}
}

// SetOnDirCreated registers a callback fired synchronously the first
// time a write creates the .emed directory. The UI uses it to refresh
// the file tree, since the watcher ignores dot entries.
func (s *Store) SetOnDirCreated(handler func()) {
	s.onDirCreated = handler
}

// Root returns the project root path this store is bound to.
func (s *Store) Root() string {
	return s.rootPath
}

// LoadStarters returns the configured starters, or an empty list when
// the project has no starters file yet.
func (s *Store) LoadStarters() ([]Starter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var starters []Starter
	if err := s.readJSON(startersFileName, &starters); err != nil {
		return nil, err
	}
	return starters, nil
}

// SaveStarters persists the starters list.
func (s *Store) SaveStarters(starters []Starter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if starters == nil {
		starters = []Starter{}
	}
	return s.writeJSON(startersFileName, starters)
}

// LoadSession returns the persisted session, or an empty session when
// the project has no session file yet.
func (s *Store) LoadSession() (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var session Session
	if err := s.readJSON(sessionFileName, &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

// SaveSession persists the session state.
func (s *Store) SaveSession(session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session.OpenFiles == nil {
		session.OpenFiles = []string{}
	}
	return s.writeJSON(sessionFileName, session)
}

// ValidateStarter checks a starter candidate against the current list.
func ValidateStarter(existing []Starter, candidate Starter) error {
	name := strings.TrimSpace(candidate.Name)
	if name == "" {
		return errors.New("starter name can not be empty")
	}
	if strings.TrimSpace(candidate.Command) == "" {
		return errors.New("starter command can not be empty")
	}
	for _, s := range existing {
		if strings.EqualFold(s.Name, name) {
			return fmt.Errorf("a starter named %q already exists", s.Name)
		}
	}
	return nil
}

func (s *Store) readJSON(fileName string, target interface{}) error {
	data, err := os.ReadFile(filepath.Join(s.rootPath, storeDirName, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, target)
}

func (s *Store) writeJSON(fileName string, value interface{}) error {
	dir := filepath.Join(s.rootPath, storeDirName)
	dirCreated := false
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		dirCreated = true
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	target := filepath.Join(dir, fileName)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		os.Remove(tmp)
		return err
	}

	if dirCreated && s.onDirCreated != nil {
		s.onDirCreated()
	}
	return nil
}
