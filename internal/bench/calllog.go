package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// callLogEnv names the per-run server-side invocation log. Unset → logging is a
// no-op, so a fixture server runs normally outside the bench.
const callLogEnv = "OZY_BENCH_CALL_LOG"

// callLogRecord is one JSONL line: which downstream server and tool ran, a
// digest of the arguments (not the raw args — compact and content-free), and a
// timestamp. This server-side record is the mode-symmetric ground truth for
// tool selection: in ozy mode the agent transcript shows only broker calls, so
// the downstream tool identity lives only here (D4).
type callLogRecord struct {
	Server     string `json:"server"`
	Tool       string `json:"tool"`
	ArgsDigest string `json:"argsDigest"`
	TS         string `json:"ts"`
}

// callLogMu serializes this process's own appends. Cross-process atomicity
// comes from O_APPEND (the fixture servers are separate processes); the records
// are small single lines.
//
// ponytail: O_APPEND + small-line writes, no file lock — upgrade to flock only
// if interleaving ever shows up in a log.
var callLogMu sync.Mutex

// logInvocation appends one record to OZY_BENCH_CALL_LOG when it is set.
func logInvocation(server, tool string, args any) {
	path := os.Getenv(callLogEnv)
	if path == "" {
		return
	}
	digest := ""
	if b, err := json.Marshal(args); err == nil {
		sum := sha256.Sum256(b)
		digest = fmt.Sprintf("%x", sum[:8])
	}
	line, err := json.Marshal(callLogRecord{
		Server:     server,
		Tool:       tool,
		ArgsDigest: digest,
		TS:         time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return
	}

	callLogMu.Lock()
	defer callLogMu.Unlock()
	//nolint:gosec // G304: path is the trusted per-run call-log env, not user input.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}

// callLogMiddleware returns receiving middleware that records every tools/call
// under server. One hook covers a server's functional and stub tools alike.
func callLogMiddleware(server string) mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			// At the receiving-middleware layer the params are the still-raw
			// CallToolParamsRaw (Arguments as json.RawMessage), not the typed
			// CallToolParams the handler later decodes into.
			if method == "tools/call" {
				if p, ok := req.GetParams().(*mcpsdk.CallToolParamsRaw); ok {
					logInvocation(server, p.Name, p.Arguments)
				}
			}
			return next(ctx, method, req)
		}
	}
}
