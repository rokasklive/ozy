package eval

import (
	"context"
	"strings"

	"github.com/rokasklive/ozy/internal/search"
)

// DiscoveryMetrics are the SPEC.md §14.2 discovery metrics for a set of cases.
// WrongServerRate and NoMatchCorrectness are only meaningful for the matching
// categories and are omitted (zero) elsewhere.
type DiscoveryMetrics struct {
	N                  int     `json:"n"`
	Top1               float64 `json:"top1"`
	Top3               float64 `json:"top3"`
	MRR                float64 `json:"mrr"`
	WrongServerRate    float64 `json:"wrongServerRate,omitempty"`
	NoMatchCorrectness float64 `json:"noMatchCorrectness,omitempty"`
}

// DiscoveryReport is the discovery family result: headline overall metrics plus a
// per-category breakdown.
type DiscoveryReport struct {
	Overall    DiscoveryMetrics            `json:"overall"`
	ByCategory map[string]DiscoveryMetrics `json:"byCategory"`
}

// RunDiscovery drives the search engine over every discovery case and computes
// the discovery metrics. It uses the engine's full ranked order (for MRR/top-3)
// and search.Decide (for no-match correctness), so it measures the same ranking
// the broker's findTool exposes. Deterministic for a fixed corpus and engine.
func RunDiscovery(ctx context.Context, engine *search.Engine, cases []DiscoveryCase) (*DiscoveryReport, error) {
	global := &discoveryAcc{}
	byCat := map[string]*discoveryAcc{}

	for _, c := range cases {
		ranking, err := engine.Find(ctx, c.Intent)
		if err != nil {
			return nil, err
		}
		decision := search.Decide(ranking)

		cat := byCat[c.Category]
		if cat == nil {
			cat = &discoveryAcc{}
			byCat[c.Category] = cat
		}

		if c.Category == CategoryNoMatch {
			correct := decision.Verdict == search.DecisionNoGoodMatch || decision.Verdict == search.DecisionCatalogEmpty
			cat.addNoMatch(correct)
			global.addNoMatch(correct)
			continue
		}

		order := rankedRefs(ranking)
		rank := firstAcceptableRank(order, c.Acceptable)
		cat.addMatchable(rank)
		global.addMatchable(rank)

		if c.Category == CategoryWrongServer {
			wrong := isWrongServerPick(order, c.Acceptable)
			cat.addWrongServer(wrong)
			global.addWrongServer(wrong)
		}
	}

	report := &DiscoveryReport{
		Overall:    global.metrics(),
		ByCategory: make(map[string]DiscoveryMetrics, len(byCat)),
	}
	for cat, acc := range byCat {
		report.ByCategory[cat] = acc.metrics()
	}
	return report, nil
}

// discoveryAcc accumulates raw counts for one scope (a category or the global
// total) and turns them into rate metrics.
type discoveryAcc struct {
	matchN  int
	top1    int
	top3    int
	mrrSum  float64
	nmN     int
	nmRight int
	wsN     int
	wsWrong int
}

func (a *discoveryAcc) addMatchable(rank int) {
	a.matchN++
	if rank == 1 {
		a.top1++
	}
	if rank >= 1 && rank <= 3 {
		a.top3++
	}
	if rank >= 1 {
		a.mrrSum += 1.0 / float64(rank)
	}
}

func (a *discoveryAcc) addNoMatch(correct bool) {
	a.nmN++
	if correct {
		a.nmRight++
	}
}

func (a *discoveryAcc) addWrongServer(wrong bool) {
	a.wsN++
	if wrong {
		a.wsWrong++
	}
}

func (a *discoveryAcc) metrics() DiscoveryMetrics {
	m := DiscoveryMetrics{}
	m.N = a.matchN + a.nmN
	if a.matchN > 0 {
		m.Top1 = float64(a.top1) / float64(a.matchN)
		m.Top3 = float64(a.top3) / float64(a.matchN)
		m.MRR = a.mrrSum / float64(a.matchN)
	}
	if a.nmN > 0 {
		m.NoMatchCorrectness = float64(a.nmRight) / float64(a.nmN)
	}
	if a.wsN > 0 {
		m.WrongServerRate = float64(a.wsWrong) / float64(a.wsN)
	}
	return m
}

// rankedRefs returns the toolRefs in fused-rank order.
func rankedRefs(r *search.Ranking) []string {
	out := make([]string, len(r.Entries))
	for i, e := range r.Entries {
		out[i] = e.Tool.ToolRef
	}
	return out
}

// firstAcceptableRank returns the 1-based rank of the first acceptable toolRef in
// order, or 0 if none of them appear.
func firstAcceptableRank(order, acceptable []string) int {
	want := make(map[string]struct{}, len(acceptable))
	for _, a := range acceptable {
		want[a] = struct{}{}
	}
	for i, ref := range order {
		if _, ok := want[ref]; ok {
			return i + 1
		}
	}
	return 0
}

// isWrongServerPick reports whether the top pick is a same-capability tool on the
// wrong server: top1 is not acceptable but shares the leading operation token
// (e.g. "search") with an acceptable tool on a different server. This is the
// failure wrong_server intents probe — right capability, wrong provider.
func isWrongServerPick(order, acceptable []string) bool {
	if len(order) == 0 {
		return false
	}
	top := order[0]
	for _, a := range acceptable {
		if top == a {
			return false
		}
	}
	topServer, topName := splitRef(top)
	for _, a := range acceptable {
		accServer, accName := splitRef(a)
		if topServer != accServer && opToken(topName) == opToken(accName) {
			return true
		}
	}
	return false
}

func splitRef(ref string) (server, name string) {
	if i := strings.IndexByte(ref, '.'); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// opToken is the leading underscore-delimited token of a downstream tool name,
// used as a coarse capability family key (e.g. "search" from "search_messages").
func opToken(name string) string {
	if i := strings.IndexByte(name, '_'); i > 0 {
		return name[:i]
	}
	return name
}
