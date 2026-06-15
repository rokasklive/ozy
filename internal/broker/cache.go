package broker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
)

// cachingBroker is a transparent decorator over a Broker. It memoizes
// findTool/describeTool results and read-only callTool results so repeated
// requests within a TTL skip the underlying search, catalog read, or downstream
// invocation. It implements Broker, so both the CLI and the MCP adapter benefit
// without either importing cache logic.
//
// Safety: callTool is cached ONLY for tools with positive read-only evidence
// (catalog.Tool.ReadOnly). Write tools and tools of unknown intent are always
// invoked live, so a cache hit can never substitute for a side effect.
//
// Cached values are produced fresh by the inner broker and consumed read-only by
// the adapters, so the shared pointer is not defensively copied.
type cachingBroker struct {
	inner Broker
	store catalog.Store
	ttl   time.Duration
	max   int

	mu      sync.Mutex
	entries map[string]cacheEntry
}

type cacheEntry struct {
	value   any
	expires time.Time
}

// NewCaching wraps inner with a result cache tuned by cfg. Callers gate the wrap
// on cfg.Enabled; a disabled cache is simply the unwrapped inner broker.
func NewCaching(inner Broker, store catalog.Store, cfg config.CacheConfig) Broker {
	return &cachingBroker{
		inner:   inner,
		store:   store,
		ttl:     cfg.TTL(),
		max:     cfg.MaxEntries,
		entries: make(map[string]cacheEntry),
	}
}

func (c *cachingBroker) FindTool(ctx context.Context, query string) (*contract.FindResult, error) {
	gen, ok := c.generation(ctx)
	if !ok {
		return c.inner.FindTool(ctx, query)
	}
	k := cacheKey("find", query, gen)
	if v, hit := c.get(k); hit {
		return v.(*contract.FindResult), nil
	}
	res, err := c.inner.FindTool(ctx, query)
	if err == nil && res != nil {
		c.put(k, res)
	}
	return res, err
}

func (c *cachingBroker) DescribeTool(ctx context.Context, toolRef string) (*contract.DescribeResult, error) {
	tool, found, err := c.store.GetTool(ctx, toolRef)
	if err != nil || !found {
		// Let the inner broker produce the proper not-found / error response.
		return c.inner.DescribeTool(ctx, toolRef)
	}
	k := cacheKey("describe", toolRef, tool.SchemaHash)
	if v, hit := c.get(k); hit {
		return v.(*contract.DescribeResult), nil
	}
	res, err := c.inner.DescribeTool(ctx, toolRef)
	if err == nil && res != nil {
		c.put(k, res)
	}
	return res, err
}

func (c *cachingBroker) CallTool(ctx context.Context, toolRef string, args map[string]any) (*contract.CallResult, error) {
	tool, found, err := c.store.GetTool(ctx, toolRef)
	if err != nil || !found || !tool.ReadOnly {
		// Default-deny: write tools and tools of unknown intent always invoke live.
		return c.inner.CallTool(ctx, toolRef, args)
	}
	argKey, err := json.Marshal(args) // map keys are emitted in sorted order, so equal args hash equally
	if err != nil {
		return c.inner.CallTool(ctx, toolRef, args)
	}
	k := cacheKey("call", toolRef, tool.SchemaHash, string(argKey))
	if v, hit := c.get(k); hit {
		return v.(*contract.CallResult), nil
	}
	res, err := c.inner.CallTool(ctx, toolRef, args)
	if err == nil && res != nil {
		c.put(k, res)
	}
	return res, err
}

// List is not cached: it is cheap and changes with the catalog.
func (c *cachingBroker) List(ctx context.Context) (*contract.ListResult, error) {
	return c.inner.List(ctx)
}

// generation returns the catalog content/generation token for findTool keys and
// whether caching should proceed. A store error skips caching for this call.
func (c *cachingBroker) generation(ctx context.Context) (string, bool) {
	t, ok, err := c.store.LastIndexedAt(ctx)
	if err != nil {
		return "", false
	}
	if !ok {
		return "never", true
	}
	return strconv.FormatInt(t.UnixNano(), 10), true
}

func cacheKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *cachingBroker) get(k string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expires) {
		delete(c.entries, k)
		return nil, false
	}
	return e.value, true
}

func (c *cachingBroker) put(k string, v any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.max > 0 && len(c.entries) >= c.max {
		c.evictLocked()
	}
	c.entries[k] = cacheEntry{value: v, expires: time.Now().Add(c.ttl)}
}

// evictLocked prunes expired entries and, if still at capacity, drops one
// arbitrary entry to make room.
// ponytail: arbitrary eviction at cap; swap for LRU only if hit-rate data shows it matters.
func (c *cachingBroker) evictLocked() {
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.expires) {
			delete(c.entries, k)
		}
	}
	if len(c.entries) < c.max {
		return
	}
	for k := range c.entries {
		delete(c.entries, k)
		return
	}
}
