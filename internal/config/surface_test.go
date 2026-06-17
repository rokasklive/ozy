package config

import "testing"

func TestLoad_CapabilityBreadcrumbDefaultsOnWhenOmitted(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{
  "version": 1,
  "mcp": {}
}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if !loaded.Resolved.Surface.CapabilityBreadcrumb {
		t.Error("capabilityBreadcrumb should default to true when the surface section is omitted")
	}
}

func TestLoad_CapabilityBreadcrumbCanBeDisabled(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{
  "version": 1,
  "mcp": {},
  "surface": { "capabilityBreadcrumb": false }
}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if loaded.Resolved.Surface.CapabilityBreadcrumb {
		t.Error("capabilityBreadcrumb should be false when explicitly disabled")
	}
}
