package eval

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"runtime/debug"
	"time"

	"github.com/rokasklive/ozy/evals"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/search"
)

// Family identifiers for scoping a run.
const (
	FamilyDiscovery   = "discovery"
	FamilyInvocation  = "invocation"
	FamilyErgonomics  = "ergonomics"
	FamilyTokens      = "tokens"
	FamilyPerformance = "performance"
	FamilyCache       = "cache"
)

// knownFamilies is the set of runnable scenario families.
var knownFamilies = map[string]bool{
	FamilyDiscovery:   true,
	FamilyInvocation:  true,
	FamilyErgonomics:  true,
	FamilyTokens:      true,
	FamilyPerformance: true,
	FamilyCache:       true,
}

// SemanticBuilder constructs a semantic provider over the corpus store for the
// real-model leg, returning the provider, a cleanup func, and an error. It lives
// behind this hook so internal/eval has no hard dependency on the sidecar; the
// CLI supplies the real (sidecar-backed) builder. When nil while Semantic is
// requested, the semantic leg is skipped (recorded), never failed.
type SemanticBuilder func(ctx context.Context, store catalog.Store, corpus *Corpus) (search.Semantic, func(), error)

// Options configures a harness run.
type Options struct {
	// Data is the corpus FS; defaults to the embedded evals.Data().
	Data fs.FS
	// OutDir is where the snapshot and scoreboard are written. Empty means do not
	// write artifacts (in-memory run only).
	OutDir string
	// Families scopes the run; empty runs all known families.
	Families []string
	// Semantic requests the real-model semantic leg (also honored via
	// OZY_EVAL_SEMANTIC=1). Requires SemanticBuilder to actually run.
	Semantic bool
	// SemanticBuilder builds the real semantic provider; see SemanticBuilder docs.
	SemanticBuilder SemanticBuilder
	// Estimator is the token estimator; defaults to DefaultEstimator.
	Estimator TokenEstimator
}

func (o *Options) wantFamily(name string) bool {
	if len(o.Families) == 0 {
		return true
	}
	for _, f := range o.Families {
		if f == name {
			return true
		}
	}
	return false
}

// Run loads the corpus, drives the requested families over the real seams,
// evaluates the gate thresholds, and returns a structured result. When OutDir is
// set it also writes the JSON snapshot and the BENCHMARKS.md scoreboard.
func Run(ctx context.Context, opts Options) (*RunResult, error) {
	if opts.Data == nil {
		opts.Data = evals.Data()
	}
	if opts.Estimator == nil {
		opts.Estimator = DefaultEstimator
	}
	for _, f := range opts.Families {
		if !knownFamilies[f] {
			return nil, fmt.Errorf("unknown eval family %q", f)
		}
	}

	corpus, err := Load(opts.Data)
	if err != nil {
		return nil, err
	}
	store, err := corpus.Store()
	if err != nil {
		return nil, err
	}

	semantic, semanticRan, cleanup := buildSemantic(ctx, opts, store, corpus)
	if cleanup != nil {
		defer cleanup()
	}
	engine := search.New(store, semantic)

	result := &RunResult{
		Schema:      SchemaVersion,
		GeneratedAt: time.Now(),
		Provenance: Provenance{
			CorpusVersion:  corpus.Catalog.Version,
			Model:          modelLabel(semanticRan),
			SemanticRan:    semanticRan,
			TokenEstimator: opts.Estimator.Name(),
			GitCommit:      gitCommit(),
			Host:           hostname(),
		},
	}

	// Hygiene runs whenever discovery is in scope (it guards the semantic gold set).
	if opts.wantFamily(FamilyDiscovery) {
		findings, herr := Hygiene(corpus)
		if herr != nil {
			return nil, herr
		}
		result.Hygiene = findings

		disco, derr := RunDiscovery(ctx, engine, corpus.Discovery)
		if derr != nil {
			return nil, derr
		}
		result.Discovery = disco
	}

	if opts.wantFamily(FamilyInvocation) && len(corpus.Invocation) > 0 {
		inv, ierr := RunInvocation(ctx, corpus)
		if ierr != nil {
			return nil, ierr
		}
		result.Invocation = inv
	}

	if opts.wantFamily(FamilyErgonomics) && len(corpus.Ergonomics) > 0 {
		erg, eerr := RunErgonomics(ctx, corpus)
		if eerr != nil {
			return nil, eerr
		}
		result.Ergonomics = erg
	}

	if opts.wantFamily(FamilyTokens) {
		econ, terr := RunTokenEconomy(ctx, corpus, opts.Estimator)
		if terr != nil {
			return nil, terr
		}
		result.TokenEconomy = econ
	}

	if opts.wantFamily(FamilyPerformance) {
		lexBroker, lberr := newCorpusBroker(corpus, nil)
		if lberr != nil {
			return nil, lberr
		}
		var semEngine *search.Engine
		if semanticRan {
			semEngine = engine
		}
		result.Performance = RunLatency(ctx, search.New(store, nil), lexBroker, semEngine)
	}

	if opts.wantFamily(FamilyCache) {
		cache, cerr := RunCacheEffectiveness(ctx, corpus, opts.Estimator)
		if cerr != nil {
			return nil, cerr
		}
		result.Cache = cache
	}

	thresholds, err := LoadThresholds(opts.Data)
	if err != nil {
		return nil, err
	}
	if result.Discovery != nil {
		result.Gates = append(result.Gates, thresholds.EvaluateDiscovery(result.Discovery, semanticRan)...)
	}
	if result.Invocation != nil {
		result.Gates = append(result.Gates, thresholds.EvaluateInvocation(result.Invocation)...)
	}
	if result.Ergonomics != nil {
		result.Gates = append(result.Gates, thresholds.EvaluateErgonomics(result.Ergonomics)...)
	}
	if result.TokenEconomy != nil {
		result.Gates = append(result.Gates, thresholds.EvaluateTokens(result.TokenEconomy)...)
	}
	if result.Cache != nil {
		result.Gates = append(result.Gates, thresholds.EvaluateCache(result.Cache)...)
	}
	result.Verdict = verdict(result.Gates)

	if opts.OutDir != "" {
		if _, err := WriteSnapshot(opts.OutDir, result); err != nil {
			return nil, err
		}
		if err := WriteScoreboard(opts.OutDir, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// buildSemantic decides whether to run the semantic leg. It returns a provider
// (nil for lexical-only), whether the leg actually ran, and a cleanup func. A
// provisioning failure is a clean skip, never a run failure (SPEC.md §10.1).
func buildSemantic(ctx context.Context, opts Options, store catalog.Store, corpus *Corpus) (search.Semantic, bool, func()) {
	want := opts.Semantic || os.Getenv("OZY_EVAL_SEMANTIC") == "1"
	if !want || opts.SemanticBuilder == nil {
		return nil, false, nil
	}
	provider, cleanup, err := opts.SemanticBuilder(ctx, store, corpus)
	if err != nil {
		return nil, false, nil
	}
	if provider == nil || !provider.Available() {
		if cleanup != nil {
			cleanup()
		}
		return nil, false, nil
	}
	return provider, true, cleanup
}

func modelLabel(semanticRan bool) string {
	if semanticRan {
		return "real-embedding (hybrid)"
	}
	return "lexical-only"
}

func gitCommit() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return "unknown"
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
