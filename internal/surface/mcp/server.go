// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/obs"
)

// ModernVersion is the stateless revision: version and capabilities travel
// in _meta on every request (specification 2026-07-28).
const ModernVersion = "2026-07-28"

// legacyVersions are the initialize-based revisions served after an
// initialize request (dual-era, ADR-0010), newest first.
var legacyVersions = []string{"2025-11-25", "2025-06-18"}

// Reserved _meta keys of the 2026-07-28 revision.
const (
	metaProtocolVersion    = "io.modelcontextprotocol/protocolVersion"
	metaClientCapabilities = "io.modelcontextprotocol/clientCapabilities"
	metaServerInfo         = "io.modelcontextprotocol/serverInfo"
	// metaTraceparent is the W3C Trace Context of the caller. SEP-414
	// (specification 2026-07-28, "Key Changes", minor change 2) keeps it
	// unprefixed so that OpenTelemetry tooling can read it.
	metaTraceparent = "traceparent"
)

// JSON-RPC and MCP error codes.
const (
	codeParseError          = -32700
	codeInvalidRequest      = -32600
	codeMethodNotFound      = -32601
	codeInvalidParams       = -32602
	codeInternalError       = -32603
	codeUnsupportedProtocol = -32022
)

// maxMessageSize bounds one JSON-RPC line.
const maxMessageSize = 16 << 20

// Server is the ship-it MCP server over stdio.
type Server struct {
	Workspace app.Workspace
	Version   string
	// Attended is set for a human interface (the VS Code extension runs
	// `ship mcp --attended`): the human reviewed the plan there, so pull
	// requests follow the settings instead of opening as drafts, and an undo
	// may change GitHub when the human confirmed it. Agents get `ship mcp`,
	// with the safeguards of ADR-0010.
	Attended bool
	// Client names the agent (ship mcp --client claude, written by ship mcp
	// install). Without it, the name the client gives in its initialize
	// request is used. Every operation made through the server records it.
	Client string
	// Log receives diagnostics; it must be stderr, never stdout.
	Log io.Writer
	// Journal records each request (method, tool, duration, trace context)
	// and keeps the black box written when a request panics; nil keeps
	// nothing.
	Journal *obs.Journal

	out io.Writer
	mu  sync.Mutex
	// legacy is the revision negotiated by initialize, "" for modern clients.
	legacy string
	// handshakeClient is the clientInfo name of the initialize request.
	handshakeClient string
}

// unknownAgent records an agent that gave no name.
const unknownAgent = "unknown"

// agentName is the name operations record: --client, else the handshake's.
func (s *Server) agentName() string {
	switch {
	case s.Client != "":
		return s.Client
	case s.handshakeClient != "":
		return s.handshakeClient
	}
	return unknownAgent
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *rpcError) Error() string { return e.Message }

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// Serve reads one JSON-RPC message per line from in and answers on out until
// in ends (the client closing stdin is the shutdown signal). Requests are
// handled one at a time: they change one repository.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.out = out
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64<<10), maxMessageSize)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
			code := codeInvalidRequest
			if err != nil {
				code = codeParseError
			}
			s.write(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: code, Message: "invalid JSON-RPC message"}})
			continue
		}
		if len(req.ID) == 0 {
			continue // notifications: initialized, cancelled… need no answer
		}
		result, err := s.serveOne(ctx, req)
		resp := response{JSONRPC: "2.0", ID: req.ID, Result: result}
		if err != nil {
			rpcErr, ok := err.(*rpcError)
			if !ok {
				rpcErr = &rpcError{Code: codeInternalError, Message: err.Error()}
			}
			resp.Result, resp.Error = nil, rpcErr
		}
		s.write(resp)
	}
	return scanner.Err()
}

// serveOne handles one request inside its trace context, journals it, and
// turns a panic into an internal error so that the server keeps serving.
func (s *Server) serveOne(ctx context.Context, req request) (result map[string]any, err error) {
	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
		Name string                     `json:"name"`
	}
	_ = json.Unmarshal(req.Params, &params)
	var traceparent string
	_ = json.Unmarshal(params.Meta[metaTraceparent], &traceparent)
	if tc, ok := obs.ParseTraceparent(traceparent); ok {
		ctx = obs.WithTrace(ctx, tc)
	}
	start := time.Now()
	defer func() {
		if p := recover(); p != nil {
			s.logger().ErrorContext(ctx, "run.panic", "panic", fmt.Sprint(p), "stack", string(debug.Stack()))
			msg := "internal error"
			if s.Journal != nil {
				if path, saveErr := s.Journal.SaveBlackBox(); saveErr == nil && path != "" {
					msg += "; details in " + path
				}
			}
			result, err = nil, &rpcError{Code: codeInternalError, Message: msg}
		}
		step := "mcp/" + req.Method
		if req.Method == "tools/call" && params.Name != "" {
			step += "/" + params.Name
		}
		attrs := []any{"step", step, "duration_ms", time.Since(start).Milliseconds()}
		level := slog.LevelInfo
		if err != nil {
			level = slog.LevelWarn
			attrs = append(attrs, "error", err.Error())
			if rpcErr, ok := err.(*rpcError); ok {
				attrs = append(attrs, "rpc_code", rpcErr.Code)
			}
		}
		s.logger().Log(ctx, level, "mcp.request", attrs...)
	}()
	return s.handle(ctx, req)
}

func (s *Server) logger() *slog.Logger {
	if s.Journal == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Journal.Logger
}

// write sends one message on one line: encoding/json escapes newlines inside
// strings, so the message never contains one (stdio framing rule).
func (s *Server) write(resp response) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(resp)
	if err != nil {
		data, _ = json.Marshal(response{JSONRPC: "2.0", ID: resp.ID, Error: &rpcError{Code: codeInternalError, Message: err.Error()}})
	}
	fmt.Fprintf(s.out, "%s\n", data)
}

// handle decides the era of a request: modern with _meta, legacy after
// initialize.
func (s *Server) handle(ctx context.Context, req request) (map[string]any, error) {
	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	_ = json.Unmarshal(req.Params, &params)
	if raw, modern := params.Meta[metaProtocolVersion]; modern {
		var version string
		_ = json.Unmarshal(raw, &version)
		if version != ModernVersion {
			return nil, &rpcError{Code: codeUnsupportedProtocol, Message: "Unsupported protocol version",
				Data: map[string]any{"supported": supportedVersions(), "requested": version}}
		}
		if _, ok := params.Meta[metaClientCapabilities]; !ok {
			return nil, &rpcError{Code: codeInvalidParams, Message: "missing _meta " + metaClientCapabilities}
		}
		result, err := s.dispatch(ctx, req, true)
		if result != nil {
			result["resultType"] = "complete"
			result["_meta"] = map[string]any{metaServerInfo: s.serverInfo()}
		}
		return result, err
	}

	switch {
	case req.Method == "initialize":
		return s.initialize(req)
	case s.legacy == "":
		// Neither modern nor initialized: name the versions, the only
		// diagnostic a legacy client can show (versioning, compatibility).
		return nil, &rpcError{Code: codeInvalidParams, Message: "missing _meta " + metaProtocolVersion + " (or initialize first)",
			Data: map[string]any{"supported": supportedVersions()}}
	}
	return s.dispatch(ctx, req, false)
}

func supportedVersions() []string { return append([]string{ModernVersion}, legacyVersions...) }

func (s *Server) serverInfo() map[string]any {
	return map[string]any{"name": "ship-it", "title": "ship-it", "version": s.Version,
		"description": "Delivers work in progress as conventional commits and draft pull requests, with undo."}
}

// initialize negotiates a legacy revision: the requested one when served,
// else the newest legacy one (2025-11-25 lifecycle).
func (s *Server) initialize(req request) (map[string]any, error) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name string `json:"name"`
		} `json:"clientInfo"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return nil, &rpcError{Code: codeInvalidParams, Message: "invalid initialize params"}
	}
	s.legacy = legacyVersions[0]
	s.handshakeClient = strings.ToLower(strings.TrimSpace(params.ClientInfo.Name))
	if slices.Contains(legacyVersions, params.ProtocolVersion) {
		s.legacy = params.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": s.legacy,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      s.serverInfo(),
		"instructions":    instructions,
	}, nil
}

func (s *Server) dispatch(ctx context.Context, req request, modern bool) (map[string]any, error) {
	switch req.Method {
	case "server/discover":
		if !modern {
			break
		}
		return map[string]any{
			"supportedVersions": supportedVersions(),
			"capabilities":      map[string]any{"tools": map[string]any{}},
			"instructions":      instructions,
			"ttlMs":             toolListTTL,
			"cacheScope":        "public",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		result := map[string]any{"tools": toolDefinitions()}
		if modern {
			result["ttlMs"], result["cacheScope"] = toolListTTL, "public"
		}
		return result, nil
	case "tools/call":
		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &call); err != nil {
			return nil, &rpcError{Code: codeInvalidParams, Message: "invalid tools/call params"}
		}
		return s.callTool(ctx, call.Name, call.Arguments)
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
}

// toolListTTL lets clients cache the tool list: it only changes with ship-it.
const toolListTTL = 3_600_000

// instructions guide the model (server/discover, initialize).
const instructions = `ship-it delivers the work in progress of this git repository the way a careful senior would.
- ship_plan shows what a delivery would do (title, branch, files, secrets, size); it changes nothing.
- ship_deliver commits the work locally on a dedicated branch, after a checkpoint. You may pass a conventional title ("feat(scope): subject") and a branch; ship-it validates them. Secrets in the changes refuse the delivery.
- ship_publish pushes the branch and opens a DRAFT pull request: a human marks it ready for review.
- ship_undo reverts the last local operation; closing a pull request is left to a human (ship undo in a terminal).
- ship_status says where the work stands and the one next action; ship_log lists what ship-it did; ship_doctor checks git, the configuration and GitHub.`
