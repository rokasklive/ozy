package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/rokasklive/ozy/internal/contract"
)

// Load reads and validates the entire corpus from fsys (typically the embedded
// evals.Data() FS). It fails fast with a path- and field-qualified error on any
// malformed file or dangling toolRef reference, so a broken dataset never reaches
// scoring. Structural validation only — semantic-leakage hygiene is reported
// separately by Hygiene so the loader stays free of the search engine.
func Load(fsys fs.FS) (*Corpus, error) {
	corpus := &Corpus{}

	if err := loadCatalog(fsys, "catalog/world.json", &corpus.Catalog); err != nil {
		return nil, err
	}
	if len(corpus.Catalog.Tools) == 0 {
		return nil, fmt.Errorf("catalog/world.json: no tools in catalog")
	}
	refs := corpus.toolRefs()
	if len(refs) != len(corpus.Catalog.Tools) {
		return nil, fmt.Errorf("catalog/world.json: duplicate toolRef in catalog")
	}
	for i, t := range corpus.Catalog.Tools {
		if t.ToolRef == "" || t.ServerID == "" || t.Name == "" {
			return nil, fmt.Errorf("catalog/world.json: tool[%d]: toolRef, serverId, and name are required", i)
		}
		if want := t.ServerID + "." + t.Name; want != t.ToolRef {
			return nil, fmt.Errorf("catalog/world.json: tool %q: toolRef must equal serverId.name (got %q)", t.ToolRef, want)
		}
	}

	disco, err := loadDiscovery(fsys, "discovery", refs)
	if err != nil {
		return nil, err
	}
	corpus.Discovery = disco
	if len(corpus.Discovery) == 0 {
		return nil, fmt.Errorf("discovery/: no discovery cases found")
	}

	inv, err := loadInvocation(fsys, "invocation", refs)
	if err != nil {
		return nil, err
	}
	corpus.Invocation = inv

	erg, err := loadErgonomics(fsys, "ergonomics")
	if err != nil {
		return nil, err
	}
	corpus.Ergonomics = erg

	return corpus, nil
}

// jsonFiles returns the sorted *.json file names under dir, or nil when the
// directory is absent so a minimal corpus (catalog + discovery only) still loads.
func jsonFiles(fsys fs.FS, dir string) []string {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// loadInvocation reads every *.json file under dir as an invocation scenario set
// and validates each scenario against the schema and the corpus catalog.
func loadInvocation(fsys fs.FS, dir string, refs map[string]struct{}) ([]InvocationScenario, error) {
	var out []InvocationScenario
	for _, name := range jsonFiles(fsys, dir) {
		file := path.Join(dir, name)
		var doc struct {
			Version   int                  `json:"version"`
			Note      string               `json:"_note"`
			Scenarios []InvocationScenario `json:"scenarios"`
		}
		if err := decodeStrict(fsys, file, &doc); err != nil {
			return nil, err
		}
		for i, s := range doc.Scenarios {
			if err := validateInvocation(s, refs); err != nil {
				return nil, fmt.Errorf("%s: scenario[%d] (%s): %w", file, i, s.Name, err)
			}
			out = append(out, s)
		}
	}
	return out, nil
}

func validateInvocation(s InvocationScenario, refs map[string]struct{}) error {
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if _, ok := refs[s.ToolRef]; !ok {
		return fmt.Errorf("toolRef %q is not present in the corpus catalog", s.ToolRef)
	}
	if strings.TrimSpace(s.Rationale) == "" {
		return fmt.Errorf("rationale is required")
	}
	switch s.ExpectedOutcome {
	case OutcomeSuccess:
		if s.ExpectedError != "" {
			return fmt.Errorf("success scenario must not declare an expectedError")
		}
	case OutcomeRepair:
		if s.ExpectedError == "" || len(s.Corrected) == 0 {
			return fmt.Errorf("repair scenario requires expectedError and corrected arguments")
		}
		if !knownErrorType(s.ExpectedError) {
			return fmt.Errorf("unknown expectedError %q", s.ExpectedError)
		}
	case OutcomeError:
		if !knownErrorType(s.ExpectedError) {
			return fmt.Errorf("error scenario requires a known expectedError, got %q", s.ExpectedError)
		}
		if s.ExpectedError == contract.ErrTypeToolSchemaChanged && len(s.LiveSchema) == 0 {
			return fmt.Errorf("TOOL_SCHEMA_CHANGED scenario requires a liveSchema to detect drift against")
		}
	default:
		return fmt.Errorf("unknown expectedOutcome %q (want success|repair|error)", s.ExpectedOutcome)
	}
	return nil
}

// loadErgonomics reads every *.json file under dir as an ergonomics case set.
// Case toolRefs are intentionally NOT validated against the catalog: several
// cases probe unknown/malformed refs to exercise the error surfaces.
func loadErgonomics(fsys fs.FS, dir string) ([]ErgonomicsCase, error) {
	var out []ErgonomicsCase
	for _, name := range jsonFiles(fsys, dir) {
		file := path.Join(dir, name)
		var doc struct {
			Version int              `json:"version"`
			Note    string           `json:"_note"`
			Cases   []ErgonomicsCase `json:"cases"`
		}
		if err := decodeStrict(fsys, file, &doc); err != nil {
			return nil, err
		}
		for i, c := range doc.Cases {
			if err := validateErgonomics(c); err != nil {
				return nil, fmt.Errorf("%s: case[%d] (%s): %w", file, i, c.Name, err)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

func validateErgonomics(c ErgonomicsCase) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(c.Rationale) == "" {
		return fmt.Errorf("rationale is required")
	}
	switch c.Kind {
	case KindFind:
		if strings.TrimSpace(c.Query) == "" {
			return fmt.Errorf("find case requires a query")
		}
	case KindDescribe, KindCall:
		if strings.TrimSpace(c.ToolRef) == "" {
			return fmt.Errorf("%s case requires a toolRef", c.Kind)
		}
	default:
		return fmt.Errorf("unknown kind %q (want find|describe|call)", c.Kind)
	}
	if c.ExpectErrorType != "" && !knownErrorType(c.ExpectErrorType) {
		return fmt.Errorf("unknown expectErrorType %q", c.ExpectErrorType)
	}
	return nil
}

// knownErrorType reports whether t is one of the structured §9.3 error types.
func knownErrorType(t string) bool {
	switch t {
	case contract.ErrTypeToolNotFound,
		contract.ErrTypeDownstreamServerOffline,
		contract.ErrTypeArgumentValidationFailed,
		contract.ErrTypeToolSchemaChanged,
		contract.ErrTypeDownstreamCallFailed,
		contract.ErrTypeAuthUnavailable,
		contract.ErrTypeConfigError:
		return true
	default:
		return false
	}
}

// decodeStrict reads name from fsys and JSON-decodes it into v with unknown
// fields rejected, wrapping any failure with the file path.
func decodeStrict(fsys fs.FS, name string, v any) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func loadCatalog(fsys fs.FS, name string, out *Catalog) error {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// loadDiscovery reads every *.jsonl file under dir, one case per non-blank line,
// validating each case's category and that its acceptable toolRefs exist.
func loadDiscovery(fsys fs.FS, dir string, refs map[string]struct{}) ([]DiscoveryCase, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("%s/: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // deterministic order

	var cases []DiscoveryCase
	for _, name := range names {
		file := path.Join(dir, name)
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		line := 0
		for sc.Scan() {
			line++
			text := strings.TrimSpace(sc.Text())
			if text == "" {
				continue
			}
			var c DiscoveryCase
			dec := json.NewDecoder(strings.NewReader(text))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&c); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", file, line, err)
			}
			if err := validateDiscoveryCase(c, refs); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", file, line, err)
			}
			cases = append(cases, c)
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	return cases, nil
}

func validateDiscoveryCase(c DiscoveryCase, refs map[string]struct{}) error {
	if strings.TrimSpace(c.Intent) == "" {
		return fmt.Errorf("intent is required")
	}
	if !validCategory(c.Category) {
		return fmt.Errorf("unknown category %q", c.Category)
	}
	if strings.TrimSpace(c.Rationale) == "" {
		return fmt.Errorf("rationale is required (every label must be justified)")
	}
	if c.Category == CategoryNoMatch {
		if len(c.Acceptable) != 0 {
			return fmt.Errorf("no_match case must have an empty acceptable list")
		}
		return nil
	}
	if len(c.Acceptable) == 0 {
		return fmt.Errorf("category %q requires at least one acceptable toolRef", c.Category)
	}
	for _, ref := range c.Acceptable {
		if _, ok := refs[ref]; !ok {
			return fmt.Errorf("acceptable toolRef %q is not present in the corpus catalog", ref)
		}
	}
	return nil
}
