package catalog

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Memory is an in-memory Store placeholder. It is safe for concurrent use and is
// empty by default, which exercises the catalog_empty path from a clean start.
type Memory struct {
	mu            sync.RWMutex
	servers       map[string]Server
	tools         map[string]Tool
	lastIndexedAt time.Time
}

// NewMemory returns an empty in-memory catalog store.
func NewMemory() *Memory {
	return &Memory{
		servers: make(map[string]Server),
		tools:   make(map[string]Tool),
	}
}

// PutServer inserts or replaces a server.
func (m *Memory) PutServer(ctx context.Context, s Server) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[s.ID] = s
	return nil
}

// PutTool inserts or replaces a tool keyed by its toolRef.
func (m *Memory) PutTool(ctx context.Context, t Tool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[t.ToolRef] = t
	return nil
}

// Servers returns all known servers in stable order.
func (m *Memory) Servers(context.Context) ([]Server, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Server, 0, len(m.servers))
	for _, s := range m.servers {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Tools returns all indexed tools in stable toolRef order.
func (m *Memory) Tools(context.Context) ([]Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Tool, 0, len(m.tools))
	for _, t := range m.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ToolRef < out[j].ToolRef })
	return out, nil
}

// GetTool resolves a tool by toolRef.
func (m *Memory) GetTool(_ context.Context, toolRef string) (Tool, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tools[toolRef]
	return t, ok, nil
}

// Stats reports counts derived from the current contents.
func (m *Memory) Stats(context.Context) (Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	stats := Stats{ConfiguredServers: len(m.servers), IndexedTools: len(m.tools)}
	for _, t := range m.tools {
		if t.Freshness == FreshnessStale {
			stats.StaleTools++
		} else {
			stats.FreshTools++
		}
	}
	return stats, nil
}

func (m *Memory) LastIndexedAt(ctx context.Context) (time.Time, bool, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	ok := !m.lastIndexedAt.IsZero()
	return m.lastIndexedAt, ok, nil
}

func (m *Memory) SetLastIndexedAt(_ context.Context, t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastIndexedAt = t
	return nil
}
