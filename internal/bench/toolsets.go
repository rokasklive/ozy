package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// This file mirrors four real public MCP servers as fixture toolsets (see
// openspec/changes/bench-retrieval-at-scale/mirror-surfaces.md, captured
// 2026-07-15). Task-critical tools are functional over baked fixture data; the
// remaining mirrored siblings are corpus-grade stubs. All calls are logged by
// the server's invocation-log middleware (D4).

// deref returns the pointed-to string, or "" for a nil pointer.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// prop builds one typed, described JSON-schema property.
func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

// objSchema builds an object JSON schema from properties and required names.
func objSchema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// addStub registers a mirrored-but-not-functional tool: a deterministic stub
// response that carries no scenario ground-truth fact.
func addStub(srv *mcpsdk.Server, name, desc string, schema map[string]any) {
	srv.AddTool(&mcpsdk.Tool{Name: name, Description: desc, InputSchema: schema},
		func(_ context.Context, _ *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return jsonResult(map[string]any{
				"ok":     true,
				"tool":   name,
				"detail": "stub response from mirrored fixture (sibling tool, not functional in the bench)",
			}), nil
		})
}

// resolveOutputPath resolves a relative path under OZY_BENCH_OUTPUT_DIR (the
// per-run agent workspace), rejecting traversal, so the grader finds artifacts.
func resolveOutputPath(rel string) (string, error) {
	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("path traversal not allowed: %s", rel)
	}
	if filepath.IsAbs(rel) {
		return rel, nil
	}
	base := os.Getenv(outputDirEnv)
	if base == "" {
		base = "."
	}
	return filepath.Join(base, rel), nil
}

// writeFixturePDF renders body to a PDF and writes it to outputPath under the
// per-run agent workspace, returning the resolved absolute path. It is the shared
// PDF backend for both the functional pdf-toolkit and the corpus
// `functional:pdf` behavior (a no-auth document rival that really produces the
// file), so the grader finds the deliverable either way (D3/4.3).
//
//nolint:gosec // G301,G306: 0755/0644 are intentional for bench agent output.
func writeFixturePDF(outputPath, title, body string) (string, error) {
	full, err := resolveOutputPath(outputPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(full, BuildPDF(PDFMeta{Title: title}, body), 0o644); err != nil {
		return "", err
	}
	return full, nil
}

// tokenize lowercases and splits on non-alphanumeric runs.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// tokenOverlap counts how many distinct query tokens appear in text — the
// deterministic ranking signal for the search fixtures.
func tokenOverlap(query, text string) int {
	textToks := map[string]bool{}
	for _, t := range tokenize(text) {
		textToks[t] = true
	}
	seen := map[string]bool{}
	n := 0
	for _, q := range tokenize(query) {
		if seen[q] {
			continue
		}
		seen[q] = true
		if textToks[q] {
			n++
		}
	}
	return n
}

// readBakedJSON reads and unmarshals a baked fixture data file into v.
//
//nolint:gosec // G304: path is under the trusted fixture dir.
func readBakedJSON(fixtureDir, name string, v any) error {
	data, err := os.ReadFile(filepath.Join(fixtureDir, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// ---------------------------------------------------------------------------
// weather (mirror: weather-mcp/weather-mcp, 17 tools)
// ---------------------------------------------------------------------------

type histWeatherInput struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	StartDate string  `json:"start_date"`
	EndDate   string  `json:"end_date"`
}

type searchLocationInput struct {
	CityName string `json:"city_name"`
}

func registerWeather(srv *mcpsdk.Server, fixtureDir string) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_historical_weather",
		Description: "Retrieve historical hourly/daily weather observations from 1940 to present for a coordinate and date range, sourced from a reanalysis archive.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ histWeatherInput) (*mcpsdk.CallToolResult, any, error) {
		var baked map[string]any
		if err := readBakedJSON(fixtureDir, "weather.json", &baked); err != nil {
			return jsonResult(map[string]any{"error": "no baked weather data: " + err.Error()}), nil, nil
		}
		return jsonResult(baked), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search_location",
		Description: "Geocode a place name to coordinates and canonical location details.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, _ searchLocationInput) (*mcpsdk.CallToolResult, any, error) {
		var baked map[string]any
		if err := readBakedJSON(fixtureDir, "weather.json", &baked); err != nil {
			return jsonResult(map[string]any{"error": "no baked weather data: " + err.Error()}), nil, nil
		}
		loc := baked["location"]
		return jsonResult(map[string]any{"results": []any{loc}}), nil, nil
	})

	// Mirrored siblings — corpus-grade stubs.
	coord := objSchema([]string{"latitude", "longitude"}, map[string]any{
		"latitude":  prop("number", "Latitude in decimal degrees."),
		"longitude": prop("number", "Longitude in decimal degrees."),
	})
	addStub(srv, "get_forecast", "Daily and hourly forecasts up to 16 days by coordinates, saved location, or city name.", coord)
	addStub(srv, "get_current_conditions", "Real-time observations: temperature, wind, heat index/wind chill, and snow depth.", coord)
	addStub(srv, "get_alerts", "Active watches, warnings, and advisories sorted by severity.", coord)
	addStub(srv, "get_weather_summary", "One-call overview combining current conditions, forecast, and active alerts.", coord)
	addStub(srv, "get_air_quality", "Air quality index scores, pollutant concentrations, UV index, and health guidance.", coord)
	addStub(srv, "get_marine_conditions", "Wave height, swell, ocean currents, and Douglas Sea Scale for coastal points.", coord)
	addStub(srv, "get_weather_imagery", "Precipitation radar and GOES satellite imagery tiles for a region.", coord)
	addStub(srv, "get_lightning_activity", "Real-time lightning strike detection with proximity-based safety assessment.", coord)
	addStub(srv, "get_river_conditions", "River gauge levels, flood stages, and streamflow for nearby monitoring stations.", coord)
	addStub(srv, "get_wildfire_info", "Active wildfires with containment, size, and proximity-based safety guidance.", coord)
	addStub(srv, "check_service_status", "Health checks for upstream weather APIs plus cache statistics.", objSchema(nil, map[string]any{}))
	addStub(srv, "save_location", "Save a place as a named alias with optional activity tags.", objSchema([]string{"name"}, map[string]any{
		"name": prop("string", "Alias to store the location under."),
	}))
	addStub(srv, "list_saved_locations", "List all saved location aliases.", objSchema(nil, map[string]any{}))
	addStub(srv, "get_saved_location", "Return details for one saved location alias.", objSchema([]string{"name"}, map[string]any{
		"name": prop("string", "Alias of the saved location."),
	}))
	addStub(srv, "remove_saved_location", "Delete a saved location alias.", objSchema([]string{"name"}, map[string]any{
		"name": prop("string", "Alias of the saved location to remove."),
	}))
}

// ---------------------------------------------------------------------------
// duckduckgo (mirror: nickclyde/duckduckgo-mcp-server, 2 tools)
// ---------------------------------------------------------------------------

type ddgSearchInput struct {
	Query      string  `json:"query"`
	MaxResults *int    `json:"max_results,omitempty"`
	Region     *string `json:"region,omitempty"`
}

type ddgFetchInput struct {
	URL        string `json:"url"`
	StartIndex *int   `json:"start_index,omitempty"`
	MaxLength  *int   `json:"max_length,omitempty"`
}

// searchEntry is one baked search-corpus result.
type searchEntry struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Content string `json:"content,omitempty"`
}

func registerDuckDuckGo(srv *mcpsdk.Server, fixtureDir string) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search",
		Description: "Perform a privacy-focused web search and return formatted results (title, URL, snippet).",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in ddgSearchInput) (*mcpsdk.CallToolResult, any, error) {
		maxResults := 0
		if in.MaxResults != nil {
			maxResults = *in.MaxResults
		}
		res, err := searchFixtureResults(fixtureDir, in.Query, maxResults)
		if err != nil {
			return jsonResult(map[string]any{"error": "no baked search corpus: " + err.Error()}), nil, nil
		}
		return jsonResult(res), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "fetch_content",
		Description: "Fetch and parse the readable text content of a web page.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in ddgFetchInput) (*mcpsdk.CallToolResult, any, error) {
		var corpus []searchEntry
		if err := readBakedJSON(fixtureDir, "search.json", &corpus); err != nil {
			return jsonResult(map[string]any{"error": "no baked search corpus: " + err.Error()}), nil, nil
		}
		for _, e := range corpus {
			if e.URL == in.URL {
				content := e.Content
				if content == "" {
					content = e.Snippet
				}
				return jsonResult(map[string]any{"url": in.URL, "content": content}), nil, nil
			}
		}
		return jsonResult(map[string]any{"url": in.URL, "content": "", "note": "no baked content for this URL"}), nil, nil
	})
}

// searchFixtureResults ranks the baked search corpus for a query and returns the
// {query, results[]} envelope, capped at max (0 = all). It is the shared search
// backend for both the functional duckduckgo toolset and the corpus
// `functional:search` behavior, so a no-auth search rival returns real results
// from the same data (D3/4.3).
func searchFixtureResults(fixtureDir, query string, max int) (map[string]any, error) {
	var corpus []searchEntry
	if err := readBakedJSON(fixtureDir, "search.json", &corpus); err != nil {
		return nil, err
	}
	ranked := rankSearch(corpus, query)
	limit := len(ranked)
	if max > 0 && max < limit {
		limit = max
	}
	out := make([]map[string]any, 0, limit)
	for _, e := range ranked[:limit] {
		out = append(out, map[string]any{"title": e.Title, "url": e.URL, "snippet": e.Snippet})
	}
	return map[string]any{"query": query, "results": out}, nil
}

// rankSearch orders the corpus by descending token overlap with the query,
// stable by original order; it never returns an empty slice for a non-empty
// corpus (the baked order is the fallback).
func rankSearch(corpus []searchEntry, query string) []searchEntry {
	type scored struct {
		e     searchEntry
		score int
		idx   int
	}
	items := make([]scored, len(corpus))
	for i, e := range corpus {
		items[i] = scored{e: e, score: tokenOverlap(query, e.Title+" "+e.Snippet+" "+e.Content), idx: i}
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].score != items[b].score {
			return items[a].score > items[b].score
		}
		return items[a].idx < items[b].idx
	})
	out := make([]searchEntry, len(items))
	for i, it := range items {
		out[i] = it.e
	}
	return out
}

// ---------------------------------------------------------------------------
// wikipedia (mirror: Rudra-ravi/wikipedia-mcp, 11 tools)
// ---------------------------------------------------------------------------

type wikiSearchInput struct {
	Query string `json:"query"`
	Limit *int   `json:"limit,omitempty"`
}

type wikiTitleInput struct {
	Title string `json:"title"`
}

// wikiArticle is one baked Wikipedia article.
type wikiArticle struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Content string `json:"content"`
}

func registerWikipedia(srv *mcpsdk.Server, fixtureDir string) {
	load := func() ([]wikiArticle, error) {
		var arts []wikiArticle
		err := readBakedJSON(fixtureDir, "wikipedia.json", &arts)
		return arts, err
	}
	findArticle := func(arts []wikiArticle, title string) *wikiArticle {
		for i := range arts {
			if strings.EqualFold(arts[i].Title, title) || strings.Contains(strings.ToLower(arts[i].Title), strings.ToLower(title)) {
				return &arts[i]
			}
		}
		return nil
	}

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search_wikipedia",
		Description: "Search Wikipedia for articles matching a query and return their titles and summaries.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in wikiSearchInput) (*mcpsdk.CallToolResult, any, error) {
		arts, err := load()
		if err != nil {
			return jsonResult(map[string]any{"error": "no baked wikipedia data: " + err.Error()}), nil, nil
		}
		sort.SliceStable(arts, func(a, b int) bool {
			return tokenOverlap(in.Query, arts[a].Title+" "+arts[a].Summary) > tokenOverlap(in.Query, arts[b].Title+" "+arts[b].Summary)
		})
		out := make([]map[string]any, 0, len(arts))
		for _, a := range arts {
			out = append(out, map[string]any{"title": a.Title, "summary": a.Summary})
		}
		return jsonResult(map[string]any{"query": in.Query, "results": out}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_article",
		Description: "Retrieve the full content of a Wikipedia article by title.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in wikiTitleInput) (*mcpsdk.CallToolResult, any, error) {
		arts, err := load()
		if err != nil {
			return jsonResult(map[string]any{"error": "no baked wikipedia data: " + err.Error()}), nil, nil
		}
		if a := findArticle(arts, in.Title); a != nil {
			return jsonResult(map[string]any{"title": a.Title, "content": a.Content}), nil, nil
		}
		return jsonResult(map[string]any{"error": "article not found", "title": in.Title}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "get_summary",
		Description: "Obtain a concise summary of a Wikipedia article by title.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in wikiTitleInput) (*mcpsdk.CallToolResult, any, error) {
		arts, err := load()
		if err != nil {
			return jsonResult(map[string]any{"error": "no baked wikipedia data: " + err.Error()}), nil, nil
		}
		if a := findArticle(arts, in.Title); a != nil {
			return jsonResult(map[string]any{"title": a.Title, "summary": a.Summary}), nil, nil
		}
		return jsonResult(map[string]any{"error": "article not found", "title": in.Title}), nil, nil
	})

	// Mirrored siblings — corpus-grade stubs.
	title := objSchema([]string{"title"}, map[string]any{"title": prop("string", "Wikipedia article title.")})
	addStub(srv, "get_sections", "Retrieve the section headings of a Wikipedia article.", title)
	addStub(srv, "get_links", "Extract the internal links contained within a Wikipedia article.", title)
	addStub(srv, "get_coordinates", "Get the geographic coordinates referenced by a Wikipedia article.", title)
	addStub(srv, "get_related_topics", "Discover topics related to an article via its links and categories.", objSchema([]string{"title"}, map[string]any{
		"title": prop("string", "Wikipedia article title."),
		"limit": prop("integer", "Maximum number of related topics to return."),
	}))
	addStub(srv, "summarize_article_for_query", "Generate a summary of an article tailored to a specific query.", objSchema([]string{"title", "query"}, map[string]any{
		"title": prop("string", "Wikipedia article title."),
		"query": prop("string", "Query to focus the summary on."),
	}))
	addStub(srv, "summarize_article_section", "Generate a summary of a specific section of an article.", objSchema([]string{"title", "section_title"}, map[string]any{
		"title":         prop("string", "Wikipedia article title."),
		"section_title": prop("string", "Section heading to summarize."),
	}))
	addStub(srv, "extract_key_facts", "Extract key facts from an article, optionally focused on a topic.", objSchema([]string{"title"}, map[string]any{
		"title":                prop("string", "Wikipedia article title."),
		"topic_within_article": prop("string", "Optional topic to focus fact extraction on."),
	}))
	addStub(srv, "test_wikipedia_connectivity", "Check connectivity to the Wikipedia API with diagnostic information.", objSchema(nil, map[string]any{}))
}

// ---------------------------------------------------------------------------
// pdf-toolkit (mirror: AryanBV/pdf-toolkit-mcp, 22 tools)
// ---------------------------------------------------------------------------

type pdfCreateInput struct {
	Text       string  `json:"text"`
	OutputPath string  `json:"outputPath"`
	PageSize   *string `json:"pageSize,omitempty"`
	Title      *string `json:"title,omitempty"`
}

type pdfCreateMDInput struct {
	Markdown   string  `json:"markdown"`
	OutputPath string  `json:"outputPath"`
	PageSize   *string `json:"pageSize,omitempty"`
	Title      *string `json:"title,omitempty"`
}

type pdfPathInput struct {
	FilePath string `json:"filePath"`
}

type pdfSearchInput struct {
	FilePath      string `json:"filePath"`
	Query         string `json:"query"`
	CaseSensitive *bool  `json:"caseSensitive,omitempty"`
}

func registerPDFToolkit(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "pdf_create",
		Description: "Generate a PDF document from plain text and write it to disk.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in pdfCreateInput) (*mcpsdk.CallToolResult, any, error) {
		full, err := writeFixturePDF(in.OutputPath, deref(in.Title), in.Text)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		return jsonResult(map[string]any{"outputPath": in.OutputPath, "path": full, "ok": true}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "pdf_create_from_markdown",
		Description: "Build a PDF from Markdown input (headings, paragraphs, lists) and write it to disk.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in pdfCreateMDInput) (*mcpsdk.CallToolResult, any, error) {
		full, err := writeFixturePDF(in.OutputPath, deref(in.Title), in.Markdown)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		return jsonResult(map[string]any{"outputPath": in.OutputPath, "path": full, "ok": true}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "pdf_extract_text",
		Description: "Extract text content from a PDF file.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in pdfPathInput) (*mcpsdk.CallToolResult, any, error) {
		full, err := resolveOutputPath(in.FilePath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		//nolint:gosec // G304: resolved under the per-run output dir.
		data, err := os.ReadFile(full)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "filePath": in.FilePath}), nil, nil
		}
		return jsonResult(map[string]any{"filePath": in.FilePath, "text": ExtractPDFText(data)}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "pdf_get_metadata",
		Description: "Retrieve document metadata (title, author, producer) and file size from a PDF.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in pdfPathInput) (*mcpsdk.CallToolResult, any, error) {
		full, err := resolveOutputPath(in.FilePath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		//nolint:gosec // G304: resolved under the per-run output dir.
		data, err := os.ReadFile(full)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "filePath": in.FilePath}), nil, nil
		}
		meta := PDFMetadata(data)
		return jsonResult(map[string]any{"filePath": in.FilePath, "title": meta.Title, "author": meta.Author, "sizeBytes": len(data)}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "pdf_search",
		Description: "Find text across a PDF and return the matching context.",
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, in pdfSearchInput) (*mcpsdk.CallToolResult, any, error) {
		full, err := resolveOutputPath(in.FilePath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		//nolint:gosec // G304: resolved under the per-run output dir.
		data, err := os.ReadFile(full)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "filePath": in.FilePath}), nil, nil
		}
		text := ExtractPDFText(data)
		hay, needle := text, in.Query
		if in.CaseSensitive == nil || !*in.CaseSensitive {
			hay, needle = strings.ToLower(text), strings.ToLower(in.Query)
		}
		found := strings.Contains(hay, needle)
		return jsonResult(map[string]any{"filePath": in.FilePath, "query": in.Query, "found": found}), nil, nil
	})

	// Mirrored siblings — corpus-grade stubs.
	filePath := objSchema([]string{"filePath"}, map[string]any{"filePath": prop("string", "Path to the PDF file.")})
	twoFiles := objSchema([]string{"filePathA", "filePathB"}, map[string]any{
		"filePathA": prop("string", "First PDF file path."),
		"filePathB": prop("string", "Second PDF file path."),
	})
	rangeSchema := objSchema([]string{"filePath", "startPage", "endPage"}, map[string]any{
		"filePath":  prop("string", "Path to the PDF file."),
		"startPage": prop("integer", "First page (1-based)."),
		"endPage":   prop("integer", "Last page (1-based)."),
	})
	addStub(srv, "pdf_get_form_fields", "List a PDF's form fields with names, types, values, and required status.", filePath)
	addStub(srv, "pdf_to_markdown", "Convert a PDF to reading-order Markdown with column clustering.", filePath)
	addStub(srv, "pdf_compare", "Produce a page-by-page text diff between two PDFs.", twoFiles)
	addStub(srv, "pdf_merge", "Combine multiple PDFs into one while preserving form fields.", objSchema([]string{"filePaths", "outputPath"}, map[string]any{
		"filePaths":  prop("array", "Ordered list of PDF file paths to merge."),
		"outputPath": prop("string", "Destination path for the merged PDF."),
	}))
	addStub(srv, "pdf_split", "Extract a page range into a new PDF.", rangeSchema)
	addStub(srv, "pdf_delete_pages", "Remove a page range from a document.", rangeSchema)
	addStub(srv, "pdf_reorder_pages", "Rearrange the pages of a PDF into a new order.", objSchema([]string{"filePath", "order"}, map[string]any{
		"filePath": prop("string", "Path to the PDF file."),
		"order":    prop("array", "New page order as a list of 1-based indices."),
	}))
	addStub(srv, "pdf_rotate_pages", "Rotate pages by 90, 180, or 270 degrees.", objSchema([]string{"filePath", "degrees"}, map[string]any{
		"filePath": prop("string", "Path to the PDF file."),
		"degrees":  prop("integer", "Rotation in degrees (90, 180, or 270)."),
	}))
	addStub(srv, "pdf_flatten", "Bake form-field values into static page content.", filePath)
	addStub(srv, "pdf_encrypt", "Apply AES-256 password protection to a PDF.", objSchema([]string{"filePath", "password"}, map[string]any{
		"filePath": prop("string", "Path to the PDF file."),
		"password": prop("string", "Password to protect the document with."),
	}))
	addStub(srv, "pdf_add_page_numbers", "Insert page numbers with configurable position and format.", filePath)
	addStub(srv, "pdf_embed_qr_code", "Embed a QR code or barcode into a PDF page.", objSchema([]string{"filePath", "data"}, map[string]any{
		"filePath": prop("string", "Path to the PDF file."),
		"data":     prop("string", "Payload to encode into the symbol."),
	}))
	addStub(srv, "pdf_create_from_template", "Generate a PDF from a named template (invoice, report, letter).", objSchema([]string{"template", "outputPath"}, map[string]any{
		"template":   prop("string", "Template name to render."),
		"outputPath": prop("string", "Destination path for the generated PDF."),
	}))
	addStub(srv, "pdf_fill_form", "Populate a PDF's form fields with values.", objSchema([]string{"filePath", "fields"}, map[string]any{
		"filePath": prop("string", "Path to the PDF file."),
		"fields":   prop("object", "Map of field names to values."),
	}))
	addStub(srv, "pdf_add_watermark", "Add a diagonal text watermark to pages.", objSchema([]string{"filePath", "text"}, map[string]any{
		"filePath": prop("string", "Path to the PDF file."),
		"text":     prop("string", "Watermark text."),
	}))
	addStub(srv, "pdf_embed_image", "Insert a PNG or JPEG image into a PDF page.", objSchema([]string{"filePath", "imagePath"}, map[string]any{
		"filePath":  prop("string", "Path to the PDF file."),
		"imagePath": prop("string", "Path to the image to embed."),
	}))
	addStub(srv, "pdf_render_pages", "Rasterize PDF pages to PNG/JPEG or inline images for vision models.", filePath)
}
