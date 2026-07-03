package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// starterConfig is the template written by `ozy init`. It uses JSONC comments,
// the opencode `mcp` shape, and {env:NAME} references instead of literal
// secrets.
const starterConfig = `{
  "$schema": "https://ozy.dev/config.json",

  // By default, ozy init writes this file to ~/.config/ozy/ozy.jsonc
  // (or the Windows user config equivalent, e.g. %AppData%\ozy\ozy.jsonc).
  // Use --config or OZY_CONFIG for an explicit project-local file.

  "mcp": {
    // Example remote downstream MCP server. Replace with your own.
    // "atlassian": {
    //   "type": "remote",
    //   "url": "https://mcp.example.com/v1/mcp",
    //   "headers": {
    //     "Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}"
    //   },
    //   "oauth": false,
    //   "enabled": true,
    //   "timeout": 5000,      // discovery/connect budget (ms) used by ozy index
    //   "callTimeout": 60000  // per-callTool budget (ms): connect + execute
    // },

    // Example local downstream MCP server.
    // "filesystem": {
    //   "type": "local",
    //   "command": ["filesystem-mcp", "--root", "."],
    //   "cwd": "/path/to/workspace",
    //   "environment": {
    //     "OZY_ROOT": "{env:OZY_ROOT}"
    //   },
    //   "enabled": true,
    //   "timeout": 5000,      // discovery/connect budget (ms) used by ozy index
    //   "callTimeout": 60000  // per-callTool budget (ms): connect + execute
    // }
  },

  "embedding": {
    "provider": "python-local",
    "required": false
  },

  "search": {
    "lexical": {
      "enabled": true
    },
    // Semantic search is on by default (hybrid lexical+semantic). It is not
    // required: if no embedding backend is available, Ozy degrades to
    // lexical-only. The installer provisions a managed Python venv for it.
    "semantic": {
      "enabled": true,
      "required": false
    }
  },

  "budgets": {
    "findTool": {
      "maxResults": 5,
      "includeFullSchemas": false
    },
    "describeTool": {
      "includeExamples": true
    },
    "callTool": {
      "maxResultBytes": 65536
    }
  },

  // The result cache memoizes findTool/describeTool results and read-only
  // callTool results within a TTL to cut redundant work. callTool is cached
  // ONLY for tools whose downstream readOnlyHint is true — write tools always
  // run live. On by default; set "enabled": false to disable.
  "cache": {
    "enabled": true,
    "ttlSeconds": 300,
    "maxEntries": 1024
  },

  // The capability breadcrumb appends a short, bounded list of your configured
  // downstream servers to the findTool description, so an agent sees what Ozy
  // can reach before its first call. On by default; set it to false to opt out.
  "surface": {
    "capabilityBreadcrumb": true
  }
}
`

// WriteStarter writes the starter configuration to path, creating parent
// directories. It refuses to overwrite an existing file so a user's config is
// never clobbered.
func WriteStarter(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("configuration already exists at %s", path)
	}
	// The config file may hold secrets, so keep it owner-private (SPEC.md §16).
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
