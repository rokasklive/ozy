// Package installer drives Ozy's setup and teardown as resumable, idempotent
// step state machines. This file holds the durable state store: the small JSON
// record that lets a rerun skip completed-and-valid steps and continue from the
// last safe point.
package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// SchemaVersion is the on-disk state schema version. Bump it on incompatible
// changes; a mismatch causes the store to start fresh rather than trust an
// unreadable record.
const SchemaVersion = 1

// StepStatus is the recorded outcome of one step.
type StepStatus string

// Recorded step outcomes.
const (
	StepPending StepStatus = "pending"
	StepDone    StepStatus = "done"
	StepFailed  StepStatus = "failed"
)

// State is the durable installer/uninstaller state persisted between runs.
type State struct {
	SchemaVersion int                  `json:"schemaVersion"`
	OzyVersion    string               `json:"ozyVersion,omitempty"`
	UpdatedAt     time.Time            `json:"updatedAt"`
	Steps         map[string]StepState `json:"steps"`
}

// StepState records one step's last outcome and any validation metadata (for
// example a failure reason, or a recorded version/checksum to revalidate on the
// next run).
type StepState struct {
	Status    StepStatus `json:"status"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Detail    string     `json:"detail,omitempty"`
}

// Done reports whether step is recorded complete.
func (s *State) Done(step string) bool {
	return s.Steps[step].Status == StepDone
}

// Mark records status and optional detail for step.
func (s *State) Mark(step string, status StepStatus, detail string) {
	if s.Steps == nil {
		s.Steps = map[string]StepState{}
	}
	s.Steps[step] = StepState{Status: status, UpdatedAt: time.Now().UTC(), Detail: detail}
}

// StateStore reads and writes State at a fixed path.
type StateStore struct {
	path string
}

// NewStateStore returns a store backed by the file at path.
func NewStateStore(path string) *StateStore {
	return &StateStore{path: path}
}

// Load reads the state file. A missing file yields a fresh empty State (not an
// error) so the first run starts clean. A schema mismatch is treated the same
// way: the recorded steps cannot be trusted, so the run starts fresh.
func (s *StateStore) Load() (*State, error) {
	data, err := os.ReadFile(s.path) //nolint:gosec // path is Ozy-owned state
	if errors.Is(err, os.ErrNotExist) {
		return freshState(), nil
	}
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.SchemaVersion != SchemaVersion {
		return freshState(), nil
	}
	if st.Steps == nil {
		st.Steps = map[string]StepState{}
	}
	return &st, nil
}

// Save atomically writes state to disk, creating parent directories. The state
// area is owner-private.
func (s *StateStore) Save(st *State) error {
	st.SchemaVersion = SchemaVersion
	st.UpdatedAt = time.Now().UTC()
	if st.Steps == nil {
		st.Steps = map[string]StepState{}
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func freshState() *State {
	return &State{SchemaVersion: SchemaVersion, Steps: map[string]StepState{}}
}
