package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// FixtureGeneratorVersion is bumped whenever the generated fixture tree changes,
// so a recorded fixture-meta.json is comparable only within a generator version.
const FixtureGeneratorVersion = "1"

// FixtureMeta holds resolved metadata after materializing a scenario fixture. It
// is serialized to fixture-meta.json beside the fixture. CulpritHash/Subject are
// populated only by scenarios that carry a code-archaeology culprit; data-only
// scenarios leave them empty and the runner's culprit check falls back.
type FixtureMeta struct {
	CulpritHash      string `json:"culpritHash"`
	CulpritSubject   string `json:"culpritSubject"`
	GeneratorVersion string `json:"generatorVersion"`
	TargetDir        string `json:"-"`
}

// ReadFixtureMeta reads fixture-meta.json from a fixture directory.
func ReadFixtureMeta(fixtureDir string) (*FixtureMeta, error) {
	//nolint:gosec // G304: fixtureDir is a controlled bench path, not user input.
	data, err := os.ReadFile(filepath.Join(fixtureDir, "fixture-meta.json"))
	if err != nil {
		return nil, fmt.Errorf("read fixture meta: %w", err)
	}
	var m FixtureMeta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("unmarshal fixture meta: %w", err)
	}
	m.TargetDir = fixtureDir
	return &m, nil
}

// GenerateCopyFixture materializes a data-only scenario fixture by copying its
// checked-in baked data (e.g. weather/search/wikipedia JSON) into targetDir and
// writing fixture-meta.json.
//
//nolint:gosec // G301,G306: permissions are intentional for bench fixture output.
func GenerateCopyFixture(srcDir, targetDir string) (*FixtureMeta, error) {
	if _, err := os.Stat(srcDir); err != nil {
		return nil, fmt.Errorf("fixture source %s: %w", srcDir, err)
	}
	if err := copyTree(srcDir, targetDir); err != nil {
		return nil, fmt.Errorf("copy fixture data: %w", err)
	}

	meta := &FixtureMeta{
		GeneratorVersion: FixtureGeneratorVersion,
		TargetDir:        targetDir,
	}
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal fixture meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "fixture-meta.json"), metaJSON, 0o644); err != nil {
		return nil, fmt.Errorf("write fixture meta: %w", err)
	}
	return meta, nil
}

// copyTree recursively copies the contents of src into dst.
//
//nolint:gosec // G301,G304,G306: bench fixture paths are trusted, not user input.
func copyTree(src, dst string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		s := filepath.Join(src, e.Name())
		d := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := copyTree(s, d); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		if err := os.WriteFile(d, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}
