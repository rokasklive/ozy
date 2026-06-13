package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// File is an atomic JSON document Store. It keeps an in-memory copy for fast
// reads and writes the whole document with temp-file plus rename after updates.
type File struct {
	mu            sync.RWMutex
	path          string
	servers       map[string]Server
	tools         map[string]Tool
	lastIndexedAt time.Time
}

type fileDocument struct {
	Servers       map[string]Server `json:"servers,omitempty"`
	Tools         map[string]Tool   `json:"tools,omitempty"`
	LastIndexedAt time.Time         `json:"lastIndexedAt,omitempty"`
}

// DefaultPath returns the durable catalog path. OZY_CATALOG overrides the
// default XDG state location.
func DefaultPath() string {
	if p := os.Getenv("OZY_CATALOG"); p != "" {
		return p
	}
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "ozy", "catalog.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "catalog.json"
	}
	return filepath.Join(home, ".local", "state", "ozy", "catalog.json")
}

// NewFile loads a durable catalog store from path, returning an empty store when
// the file does not exist yet.
func NewFile(path string) (*File, error) {
	if path == "" {
		return nil, errors.New("catalog path is empty")
	}
	store := &File{
		path:    path,
		servers: make(map[string]Server),
		tools:   make(map[string]Tool),
	}
	// #nosec G304 -- catalog path is the user-selected local state path.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return store, nil
		}
		return nil, fmt.Errorf("read catalog %s: %w", path, err)
	}
	var doc fileDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode catalog %s: %w", path, err)
	}
	for id, s := range doc.Servers {
		store.servers[id] = s
	}
	for ref, t := range doc.Tools {
		store.tools[ref] = t
	}
	store.lastIndexedAt = doc.LastIndexedAt
	return store, nil
}

// PutServer inserts or replaces a server and persists the document.
func (f *File) PutServer(ctx context.Context, s Server) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.servers[s.ID] = s
	return f.persistLocked()
}

// PutTool inserts or replaces a tool keyed by its toolRef and persists the
// document.
func (f *File) PutTool(ctx context.Context, t Tool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tools[t.ToolRef] = t
	return f.persistLocked()
}

// Servers returns all known servers in stable order.
func (f *File) Servers(ctx context.Context) ([]Server, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]Server, 0, len(f.servers))
	for _, s := range f.servers {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Tools returns all indexed tools in stable toolRef order.
func (f *File) Tools(ctx context.Context) ([]Tool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]Tool, 0, len(f.tools))
	for _, t := range f.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToolRef < out[j].ToolRef })
	return out, nil
}

// GetTool resolves a tool by toolRef.
func (f *File) GetTool(ctx context.Context, toolRef string) (Tool, bool, error) {
	if err := ctx.Err(); err != nil {
		return Tool{}, false, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	t, ok := f.tools[toolRef]
	return t, ok, nil
}

// Stats reports counts derived from the current contents.
func (f *File) Stats(ctx context.Context) (Stats, error) {
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	stats := Stats{ConfiguredServers: len(f.servers), IndexedTools: len(f.tools)}
	for _, t := range f.tools {
		if t.Freshness == FreshnessStale {
			stats.StaleTools++
		} else {
			stats.FreshTools++
		}
	}
	return stats, nil
}

func (f *File) LastIndexedAt(ctx context.Context) (time.Time, bool, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, false, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	ok := !f.lastIndexedAt.IsZero()
	return f.lastIndexedAt, ok, nil
}

func (f *File) SetLastIndexedAt(ctx context.Context, t time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastIndexedAt = t
	return f.persistLocked()
}

func (f *File) persistLocked() error {
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create catalog directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".catalog-*.tmp")
	if err != nil {
		return fmt.Errorf("create catalog temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(fileDocument{Servers: f.servers, Tools: f.tools, LastIndexedAt: f.lastIndexedAt}); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("encode catalog: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close catalog temp file: %w", err)
	}
	if err := os.Rename(tmpName, f.path); err != nil {
		return fmt.Errorf("replace catalog: %w", err)
	}
	return nil
}
