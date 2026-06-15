package bench

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// jsonResult marshals v into a CallToolResult containing JSON text content.
func jsonResult(v any) *mcpsdk.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		data = []byte(`{"error":"failed to encode response"}`)
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(data)}},
	}
}

// safePath returns the absolute path resolved under fixtureDir, rejecting
// paths that contain ".." to prevent directory traversal.
func safePath(fixtureDir, relPath string) (string, error) {
	if strings.Contains(relPath, "..") {
		return "", fmt.Errorf("path traversal not allowed: %s", relPath)
	}
	return filepath.Join(fixtureDir, relPath), nil
}

// execOutput runs cmd and returns trimmed stdout, or stderr on failure.
func execOutput(cmd *exec.Cmd) (string, error) {
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("%s: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runRg runs ripgrep with the given args in dir and returns the output.
// ripgrep exits 1 when there are no matches — a valid empty result, not an
// error; only exit code >= 2 is a real failure.
func runRg(dir string, args ...string) (string, error) {
	cmd := exec.Command("rg", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				return "", nil
			}
			return "", fmt.Errorf("%s: %s", err, string(exitErr.Stderr))
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runGit runs git with the given args in dir and returns the output.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return execOutput(cmd)
}

// ServeMCP creates an MCP server for the given toolset and serves it over
// stdio. fixtureDir is used by toolsets that need access to the fixture
// directory (code-search, git, incident-db, filesystem).
func ServeMCP(toolset, fixtureDir string) error {
	srv, err := newMCPServer(toolset, fixtureDir)
	if err != nil {
		return fmt.Errorf("create mcp server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return srv.Run(ctx, &mcpsdk.StdioTransport{})
}

// newMCPServer creates and configures an *mcpsdk.Server for the given
// toolset, ready to run with any transport (stdio or in-memory for tests).
func newMCPServer(toolset, fixtureDir string) (*mcpsdk.Server, error) {
	name := "ozy-bench-" + toolset
	title := fmt.Sprintf("Ozy Bench %s Fixture MCP Server", toolset)
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    name,
		Title:   title,
		Version: "0.1.0",
	}, nil)

	switch toolset {
	case "code-search":
		registerCodeSearch(srv, fixtureDir)
	case "git":
		registerGit(srv, fixtureDir)
	case "incident-db":
		registerIncidentDB(srv, fixtureDir)
	case "filesystem":
		registerFilesystem(srv, fixtureDir)
	case "time":
		registerTime(srv)
	case "memory":
		registerMemory(srv)
	case "notes":
		registerNotes(srv)
	default:
		return nil, fmt.Errorf("unknown toolset: %s (valid: code-search, git, incident-db, filesystem, time, memory, notes)", toolset)
	}

	return srv, nil
}

// ---------------------------------------------------------------------------
// code-search toolset
// ---------------------------------------------------------------------------

type searchTextInput struct {
	Query string `json:"query"`
}

type searchSymbolInput struct {
	Symbol string `json:"symbol"`
}

type readFileInput struct {
	Path string `json:"path"`
}

type findReferencesInput struct {
	Symbol string `json:"symbol"`
}

func registerCodeSearch(srv *mcpsdk.Server, fixtureDir string) {
	searchRoot := filepath.Join(fixtureDir, "src")

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search_text",
		Title:       "Search text in fixture source files",
		Description: "Case-insensitive full-text search across all source files using ripgrep. Returns matching lines with file paths and line numbers.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in searchTextInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := runRg(searchRoot, "-n", "-i", "--no-heading", in.Query)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "query": in.Query}), nil, nil
		}
		return jsonResult(map[string]any{"query": in.Query, "results": out}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search_symbol",
		Title:       "Search for a symbol in fixture source files",
		Description: "Word-boundary symbol search across all source files using ripgrep. Returns matching lines with file paths and line numbers.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in searchSymbolInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := runRg(searchRoot, "-n", "-w", in.Symbol)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "symbol": in.Symbol}), nil, nil
		}
		return jsonResult(map[string]any{"symbol": in.Symbol, "results": out}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "read_file",
		Title:       "Read a file from the fixture directory",
		Description: "Read and return the full content of a file within the fixture directory. Path is relative to the fixture root.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in readFileInput) (*mcpsdk.CallToolResult, any, error) {
		fullPath, err := safePath(fixtureDir, in.Path)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "path": in.Path}), nil, nil
		}
		return jsonResult(map[string]any{"path": in.Path, "content": string(data)}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "find_references",
		Title:       "Find all references to a symbol",
		Description: "Search for all occurrences of a symbol across the fixture source tree using word-boundary ripgrep. Returns all matching lines with file paths.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in findReferencesInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := runRg(searchRoot, "-n", "-w", in.Symbol)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "symbol": in.Symbol}), nil, nil
		}
		return jsonResult(map[string]any{"symbol": in.Symbol, "references": out}), nil, nil
	})
}

// ---------------------------------------------------------------------------
// git toolset
// ---------------------------------------------------------------------------

type gitLogInput struct {
	Path     *string `json:"path,omitempty"`
	MaxCount *int    `json:"maxCount,omitempty"`
}

type gitShowInput struct {
	CommitRef string `json:"commitRef"`
}

type gitBlameInput struct {
	FilePath string `json:"filePath"`
}

type gitDiffInput struct {
	CommitRef *string `json:"commitRef,omitempty"`
}

func registerGit(srv *mcpsdk.Server, fixtureDir string) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "git_log",
		Title:       "Show git commit log",
		Description: "Show the git commit log in one-line format. Optionally restrict to a path and limit the number of entries.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in gitLogInput) (*mcpsdk.CallToolResult, any, error) {
		args := []string{"log", "--oneline"}
		if in.MaxCount != nil && *in.MaxCount > 0 {
			args = append(args, fmt.Sprintf("-%d", *in.MaxCount))
		}
		if in.Path != nil && *in.Path != "" {
			args = append(args, "--", *in.Path)
		}
		out, err := runGit(fixtureDir, args...)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		return jsonResult(map[string]any{"commits": out}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "git_show",
		Title:       "Show details of a git commit",
		Description: "Show the full diff and commit message for a given commit reference.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in gitShowInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := runGit(fixtureDir, "show", in.CommitRef)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "commitRef": in.CommitRef}), nil, nil
		}
		return jsonResult(map[string]any{"commitRef": in.CommitRef, "diff": out}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "git_blame",
		Title:       "Show git blame for a file",
		Description: "Show line-by-line authorship information for a file in the repository.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in gitBlameInput) (*mcpsdk.CallToolResult, any, error) {
		out, err := runGit(fixtureDir, "blame", in.FilePath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "filePath": in.FilePath}), nil, nil
		}
		return jsonResult(map[string]any{"filePath": in.FilePath, "blame": out}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "git_diff",
		Title:       "Show git diff",
		Description: "Show the working tree diff, or the diff for a specific commit reference.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in gitDiffInput) (*mcpsdk.CallToolResult, any, error) {
		args := []string{"diff"}
		if in.CommitRef != nil && *in.CommitRef != "" {
			args = append(args, *in.CommitRef)
		}
		out, err := runGit(fixtureDir, args...)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		return jsonResult(map[string]any{"diff": out}), nil, nil
	})
}

// ---------------------------------------------------------------------------
// incident-db toolset
// ---------------------------------------------------------------------------

type listTablesInput struct{}

type describeTableInput struct {
	TableName string `json:"tableName"`
}

type queryReadonlyInput struct {
	Query string `json:"query"`
}

// isReadOnlySQL returns true if the statement is a read-only SQL statement
// (SELECT, PRAGMA, or EXPLAIN).
func isReadOnlySQL(query string) bool {
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	if strings.HasPrefix(trimmed, "SELECT") {
		return true
	}
	if strings.HasPrefix(trimmed, "PRAGMA") {
		return true
	}
	if strings.HasPrefix(trimmed, "EXPLAIN") {
		return true
	}
	return false
}

// forbiddenWriteStmt checks if the query contains forbidden write operations.
func forbiddenWriteStmt(query string) bool {
	upper := strings.ToUpper(query)
	for _, keyword := range []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER"} {
		if strings.Contains(upper, keyword) {
			return true
		}
	}
	return false
}

func registerIncidentDB(srv *mcpsdk.Server, fixtureDir string) {
	dbPath := filepath.Join(fixtureDir, "db", "incident.sqlite")

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "list_tables",
		Title:       "List all tables in the incident database",
		Description: "Return a list of all table names in the incident SQLite database.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ listTablesInput) (*mcpsdk.CallToolResult, any, error) {
		db, err := sql.Open("sqlite", dbPath+"?mode=ro")
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("open db: %v", err)}), nil, nil
		}
		defer db.Close()

		rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("query tables: %v", err)}), nil, nil
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				return jsonResult(map[string]any{"error": fmt.Sprintf("scan: %v", err)}), nil, nil
			}
			tables = append(tables, name)
		}
		if err := rows.Err(); err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("rows: %v", err)}), nil, nil
		}
		return jsonResult(map[string]any{"tables": tables}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "describe_table",
		Title:       "Describe a table's schema",
		Description: "Return column information for a table using PRAGMA table_info.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in describeTableInput) (*mcpsdk.CallToolResult, any, error) {
		db, err := sql.Open("sqlite", dbPath+"?mode=ro")
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("open db: %v", err)}), nil, nil
		}
		defer db.Close()

		rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", in.TableName))
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("describe table: %v", err), "tableName": in.TableName}), nil, nil
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("columns: %v", err)}), nil, nil
		}
		var results []map[string]any
		for rows.Next() {
			vals := make([]any, len(cols))
			valPtrs := make([]any, len(cols))
			for i := range vals {
				valPtrs[i] = &vals[i]
			}
			if err := rows.Scan(valPtrs...); err != nil {
				return jsonResult(map[string]any{"error": fmt.Sprintf("scan: %v", err)}), nil, nil
			}
			row := make(map[string]any, len(cols))
			for i, col := range cols {
				row[col] = fmt.Sprintf("%v", vals[i])
			}
			results = append(results, row)
		}
		if err := rows.Err(); err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("rows: %v", err)}), nil, nil
		}
		return jsonResult(map[string]any{"tableName": in.TableName, "columns": results}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "query_readonly",
		Title:       "Execute a read-only SQL query",
		Description: "Execute a SELECT, PRAGMA, or EXPLAIN query against the incident database. Write operations are rejected.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in queryReadonlyInput) (*mcpsdk.CallToolResult, any, error) {
		if forbiddenWriteStmt(in.Query) {
			return jsonResult(map[string]any{
				"error":  "write statements are not allowed",
				"detail": "only SELECT, PRAGMA, and EXPLAIN are permitted",
			}), nil, nil
		}
		if !isReadOnlySQL(in.Query) {
			return jsonResult(map[string]any{
				"error":  "unrecognized statement type",
				"detail": "only SELECT, PRAGMA, and EXPLAIN are permitted",
			}), nil, nil
		}

		db, err := sql.Open("sqlite", dbPath+"?mode=ro")
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("open db: %v", err)}), nil, nil
		}
		defer db.Close()

		rows, err := db.QueryContext(ctx, in.Query)
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("query: %v", err), "query": in.Query}), nil, nil
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("columns: %v", err)}), nil, nil
		}
		var results []map[string]any
		for rows.Next() {
			vals := make([]any, len(cols))
			valPtrs := make([]any, len(cols))
			for i := range vals {
				valPtrs[i] = &vals[i]
			}
			if err := rows.Scan(valPtrs...); err != nil {
				return jsonResult(map[string]any{"error": fmt.Sprintf("scan: %v", err)}), nil, nil
			}
			row := make(map[string]any, len(cols))
			for i, col := range cols {
				row[col] = fmt.Sprintf("%v", vals[i])
			}
			results = append(results, row)
		}
		if err := rows.Err(); err != nil {
			return jsonResult(map[string]any{"error": fmt.Sprintf("rows: %v", err)}), nil, nil
		}
		return jsonResult(map[string]any{"columns": cols, "rows": results}), nil, nil
	})
}

// ---------------------------------------------------------------------------
// filesystem toolset
// ---------------------------------------------------------------------------

type fsReadFileInput struct {
	Path string `json:"path"`
}

type listDirInput struct {
	Path string `json:"path"`
}

func registerFilesystem(srv *mcpsdk.Server, fixtureDir string) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "read_file",
		Title:       "Read a file from the fixture directory",
		Description: "Read and return the full content of a file within the fixture directory. Path is relative to the fixture root.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in fsReadFileInput) (*mcpsdk.CallToolResult, any, error) {
		fullPath, err := safePath(fixtureDir, in.Path)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "path": in.Path}), nil, nil
		}
		return jsonResult(map[string]any{"path": in.Path, "content": string(data)}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "list_dir",
		Title:       "List directory contents",
		Description: "List files and directories within a path in the fixture directory. Path is relative to the fixture root.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in listDirInput) (*mcpsdk.CallToolResult, any, error) {
		fullPath, err := safePath(fixtureDir, in.Path)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error()}), nil, nil
		}
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			return jsonResult(map[string]any{"error": err.Error(), "path": in.Path}), nil, nil
		}
		var result []map[string]any
		for _, e := range entries {
			result = append(result, map[string]any{
				"name":  e.Name(),
				"isDir": e.IsDir(),
			})
		}
		return jsonResult(map[string]any{"path": in.Path, "entries": result}), nil, nil
	})
}

// ---------------------------------------------------------------------------
// time toolset
// ---------------------------------------------------------------------------

type currentTimeInput struct{}

type convertTimezoneInput struct {
	Time   string `json:"time"`
	FromTz string `json:"fromTz"`
	ToTz   string `json:"toTz"`
}

func registerTime(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "current_time",
		Title:       "Get current UTC time",
		Description: "Returns the current time in UTC as an ISO 8601 string.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, _ currentTimeInput) (*mcpsdk.CallToolResult, any, error) {
		now := time.Now().UTC().Format(time.RFC3339)
		return jsonResult(map[string]any{"time": now, "timezone": "UTC"}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "convert_timezone",
		Title:       "Convert time between timezones",
		Description: "Convert a time string from one timezone to another. Accepts ISO 8601 time strings.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in convertTimezoneInput) (*mcpsdk.CallToolResult, any, error) {
		// Simple/fake conversion: parse the time, apply offset from timezone
		// abbreviation or hour offset, and return the shifted time.
		fromOffset := parseTzOffset(in.FromTz)
		toOffset := parseTzOffset(in.ToTz)

		t, err := time.Parse(time.RFC3339, in.Time)
		if err != nil {
			// Try a simpler format.
			t, err = time.Parse("2006-01-02T15:04:05", in.Time)
			if err != nil {
				return jsonResult(map[string]any{"error": fmt.Sprintf("cannot parse time: %v", err)}), nil, nil
			}
		}

		// Remove fromTz offset, add toTz offset.
		adjusted := t.Add(time.Duration(-fromOffset) * time.Hour).Add(time.Duration(toOffset) * time.Hour)
		return jsonResult(map[string]any{
			"original":  in.Time,
			"fromTz":    in.FromTz,
			"toTz":      in.ToTz,
			"converted": adjusted.Format(time.RFC3339),
		}), nil, nil
	})
}

// parseTzOffset converts a timezone string to an hour offset. Handles common
// abbreviations and UTC offset strings like "+05:30" or "-04:00".
func parseTzOffset(tz string) int {
	tz = strings.TrimSpace(tz)
	tzUpper := strings.ToUpper(tz)

	offsets := map[string]int{
		"UTC": 0, "GMT": 0, "Z": 0,
		"EST": -5, "EDT": -4,
		"CST": -6, "CDT": -5,
		"MST": -7, "MDT": -6,
		"PST": -8, "PDT": -7,
		"CET": 1, "CEST": 2,
		"EET": 2, "EEST": 3,
		"IST":  5, // India: +05:30 (simplified to 5)
		"JST":  9,
		"AEST": 10,
	}
	if offset, ok := offsets[tzUpper]; ok {
		return offset
	}

	// Try parsing as ±HH:MM or ±HH.
	if strings.HasPrefix(tz, "+") || strings.HasPrefix(tz, "-") {
		var sign int
		if strings.HasPrefix(tz, "-") {
			sign = -1
		} else {
			sign = 1
		}
		cleanTz := strings.TrimPrefix(tz, "+")
		cleanTz = strings.TrimPrefix(cleanTz, "-")

		parts := strings.Split(cleanTz, ":")
		if len(parts) == 2 {
			h := 0
			m := 0
			fmt.Sscanf(parts[0], "%d", &h)
			fmt.Sscanf(parts[1], "%d", &m)
			return sign * h
		}
		h := 0
		fmt.Sscanf(cleanTz, "%d", &h)
		return sign * h
	}

	return 0
}

// ---------------------------------------------------------------------------
// memory toolset
// ---------------------------------------------------------------------------

type searchMemoryInput struct {
	Query string `json:"query"`
}

type storeMemoryInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var (
	memoryStore   = make(map[string]string)
	memoryStoreMu sync.RWMutex
)

func registerMemory(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "search_memory",
		Title:       "Search the in-memory store",
		Description: "Search keys and values in the in-process memory store for a matching substring.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in searchMemoryInput) (*mcpsdk.CallToolResult, any, error) {
		memoryStoreMu.RLock()
		defer memoryStoreMu.RUnlock()

		var matches []map[string]string
		for k, v := range memoryStore {
			if strings.Contains(k, in.Query) || strings.Contains(v, in.Query) {
				matches = append(matches, map[string]string{"key": k, "value": v})
			}
		}
		return jsonResult(map[string]any{"query": in.Query, "matches": matches}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "store_memory",
		Title:       "Store a key-value pair in memory",
		Description: "Store a key-value pair in the in-process memory store.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in storeMemoryInput) (*mcpsdk.CallToolResult, any, error) {
		memoryStoreMu.Lock()
		memoryStore[in.Key] = in.Value
		memoryStoreMu.Unlock()
		return jsonResult(map[string]any{"stored": true, "key": in.Key, "value": in.Value}), nil, nil
	})
}

// ---------------------------------------------------------------------------
// notes toolset
// ---------------------------------------------------------------------------

type createPlanInput struct {
	Title string   `json:"title"`
	Steps []string `json:"steps"`
}

type appendNoteInput struct {
	Text string `json:"text"`
}

func registerNotes(srv *mcpsdk.Server) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "create_plan",
		Title:       "Create a plan with steps",
		Description: "Create a formatted plan with a title and ordered steps.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in createPlanInput) (*mcpsdk.CallToolResult, any, error) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("# %s\n\n", in.Title))
		for i, step := range in.Steps {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
		}
		return jsonResult(map[string]any{
			"plan":  sb.String(),
			"title": in.Title,
			"steps": in.Steps,
			"count": len(in.Steps),
		}), nil, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "append_note",
		Title:       "Append a note",
		Description: "Record a text note and return confirmation.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, in appendNoteInput) (*mcpsdk.CallToolResult, any, error) {
		now := time.Now().UTC().Format(time.RFC3339)
		return jsonResult(map[string]any{
			"noted":     true,
			"text":      in.Text,
			"timestamp": now,
		}), nil, nil
	})
}

// ---------------------------------------------------------------------------
// CLI command
// ---------------------------------------------------------------------------

func (a *app) mcpCmd() *cobra.Command {
	var toolset string
	var fixtureDir string
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve a fixture MCP server",
		Long:  "Serve a parameterized fixture MCP server for the selected toolset over stdio.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if toolset == "" {
				return fmt.Errorf("--toolset is required (valid: code-search, git, incident-db, filesystem, time, memory, notes)")
			}
			if fixtureDir == "" {
				fixtureDir = os.Getenv("OZY_BENCH_FIXTURE_DIR")
			}
			return ServeMCP(toolset, fixtureDir)
		},
	}
	cmd.Flags().StringVar(&toolset, "toolset", "", "toolset to serve: code-search, git, incident-db, filesystem, time, memory, or notes")
	cmd.Flags().StringVar(&fixtureDir, "fixture-dir", "", "path to the fixture directory (required for code-search, git, incident-db, filesystem)")
	return cmd
}
