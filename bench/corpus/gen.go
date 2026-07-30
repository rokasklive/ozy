//go:build ignore

// Command gen authors the bench tool corpus: it emits one bench/corpus/<server>.json
// per fixture MCP server from the compact definitions below. The emitted JSON is
// the checked-in, reviewable data (task 2.3); this file is the authoring source.
// Run from the repo root:
//
//	go run bench/corpus/gen.go
//
// The corpus mirrors the tool surfaces of well-known real MCP servers (github,
// slack, jira, stripe, …) and synthesizes the rest in the same production tone,
// including the mandatory near-miss distractor families (rival search, weather
// lookalikes, document/PDF rivals) so retrieval must pick the exact tool.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const outDir = "bench/corpus"

// P is a schema property shorthand.
type P struct{ Type, Desc string }

func s(d string) P { return P{"string", d} }
func i(d string) P { return P{"integer", d} }
func b(d string) P { return P{"boolean", d} }
func a(d string) P { return P{"array", d} }
func o(d string) P { return P{"object", d} }
func n(d string) P { return P{"number", d} }

type tool struct {
	name, desc string
	req        []string
	props      map[string]P
}

func t(name, desc string, req []string, props map[string]P) tool {
	return tool{name, desc, req, props}
}

type server struct {
	name, mirror string
	tools        []tool
}

// no-arg helper for tools with an empty schema.
var none = map[string]P{}

func main() {
	total := 0
	for _, sv := range servers {
		if err := emit(sv); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		total += len(sv.tools)
	}
	fmt.Printf("wrote %d servers, %d tools to %s\n", len(servers), total, outDir)
}

// toolBehavior assigns the real-world behavior tier (D3) to a corpus tool so a
// same-capability rival of the scenario's canonical tools behaves like the real
// service it mirrors: auth-gated rivals return an authentication-required error;
// no-auth same-capability rivals are functional. Everything else stays a stub
// (empty behavior). Kept here (not in the data) so the policy is reviewable in
// one place.
func toolBehavior(srv, name string) string {
	switch srv {
	case "brave-search", "bing-search", "kagi-search", "climate-analytics":
		// Web-search rivals (API-key / subscription gated) and climate-data APIs
		// (registration/keys required) — a keyless agent cannot use them.
		return "auth_error"
	case "doc-converter":
		if name == "convert_html_to_pdf" {
			return "functional:pdf" // no-auth; renders provided HTML to a real PDF
		}
	case "office-export":
		if name == "export_to_pdf" {
			return "functional:pdf" // no-auth; exports provided content to a real PDF
		}
	}
	return ""
}

func emit(sv server) error {
	type jtool struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"inputSchema"`
		Behavior    string         `json:"behavior,omitempty"`
	}
	type jserver struct {
		Server       string  `json:"server"`
		MirrorSource string  `json:"mirrorSource"`
		Tools        []jtool `json:"tools"`
	}
	out := jserver{Server: sv.name, MirrorSource: sv.mirror}
	for _, tl := range sv.tools {
		props := map[string]any{}
		for k, v := range tl.props {
			props[k] = map[string]any{"type": v.Type, "description": v.Desc}
		}
		schema := map[string]any{"type": "object", "properties": props}
		if len(tl.req) > 0 {
			schema["required"] = tl.req
		}
		out.Tools = append(out.Tools, jtool{tl.name, tl.desc, schema, toolBehavior(sv.name, tl.name)})
	}
	sort.Slice(out.Tools, func(x, y int) bool { return out.Tools[x].Name < out.Tools[y].Name })
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, sv.name+".json"), append(data, '\n'), 0o644)
}
