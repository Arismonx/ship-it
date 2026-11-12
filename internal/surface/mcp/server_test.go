// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/GregoireF/ship-it/internal/app"
	"github.com/GregoireF/ship-it/internal/core/oplog"
	"github.com/GregoireF/ship-it/internal/schema"
	"github.com/GregoireF/ship-it/internal/testkit"
)

const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{},"io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"}}`

func modern(id int, method, params string) string {
	if params != "" {
		params += ","
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":%q,"params":{%s%s}}`, id, method, params, meta)
}

type harness struct {
	ws    app.Workspace
	calls *[]string
	log   *testkit.FakeOperationLog
}

func newHarness() harness {
	var calls []string
	git := testkit.HealthyRepository("main", "internal/app/run.go")
	git.DiffText = "diff --git a/internal/app/run.go b/internal/app/run.go\n--- a/internal/app/run.go\n+++ b/internal/app/run.go\n@@ -1 +1 @@\n-a\n+b\n"
	git.Calls = &calls
	log := &testkit.FakeOperationLog{}
	ws := app.Workspace{Git: git, Checkpoints: &testkit.FakeCheckpointStore{}, Operations: log, Forge: &testkit.FakeForge{}, OperatingSystem: "linux"}
	return harness{ws: ws, calls: &calls, log: log}
}

// exchange sends lines and returns the decoded responses, in order.
func (h harness) exchange(t *testing.T, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	srv := &Server{Workspace: h.ws, Version: "1.2.3"}
	if err := srv.Serve(context.Background(), strings.NewReader(strings.Join(lines, "\n")+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	var responses []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("not one JSON message per line: %q", line)
		}
		responses = append(responses, r)
	}
	return responses
}

func result(t *testing.T, r map[string]any) map[string]any {
	t.Helper()
	res, ok := r["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", r)
	}
	return res
}

func errorCode(r map[string]any) float64 {
	e, _ := r["error"].(map[string]any)
	code, _ := e["code"].(float64)
	return code
}

func TestModernDiscovery(t *testing.T) {
	r := newHarness().exchange(t, modern(1, "server/discover", ""))
	res := result(t, r[0])
	if res["resultType"] != "complete" || fmt.Sprint(res["supportedVersions"]) != "[2026-07-28 2025-11-25 2025-06-18]" {
		t.Fatalf("discover = %v", res)
	}
	info := res["_meta"].(map[string]any)["io.modelcontextprotocol/serverInfo"].(map[string]any)
	if info["name"] != "ship-it" || info["version"] != "1.2.3" {
		t.Errorf("serverInfo = %v", info)
	}
}

func TestProtocolErrors(t *testing.T) {
	r := newHarness().exchange(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"1900-01-01","io.modelcontextprotocol/clientCapabilities":{}}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		modern(4, "tools/call", `"name":"nope","arguments":{}`),
		modern(5, "no/such/method", ""),
		`not json`,
	)
	want := []float64{-32022, -32602, -32602, -32602, -32601, -32700}
	for i, code := range want {
		if got := errorCode(r[i]); got != code {
			t.Errorf("response %d: code %v, want %v (%v)", i+1, got, code, r[i])
		}
	}
	data := r[0]["error"].(map[string]any)["data"].(map[string]any)
	if data["requested"] != "1900-01-01" {
		t.Errorf("data = %v", data)
	}
}

func TestLegacySession(t *testing.T) {
	r := newHarness().exchange(t,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"old","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
	)
	if len(r) != 3 {
		t.Fatalf("notifications get no answer: %d responses", len(r))
	}
	if v := result(t, r[0])["protocolVersion"]; v != "2025-06-18" {
		t.Errorf("negotiated %v", v)
	}
	list := result(t, r[1])
	if _, modernField := list["resultType"]; modernField || len(list["tools"].([]any)) != len(tools) {
		t.Errorf("legacy tools/list = %v", list)
	}
}

func TestToolsListIsDeterministicAndDescribed(t *testing.T) {
	r := newHarness().exchange(t, modern(1, "tools/list", ""), modern(2, "tools/list", ""))
	a, _ := json.Marshal(result(t, r[0])["tools"])
	b, _ := json.Marshal(result(t, r[1])["tools"])
	if !bytes.Equal(a, b) {
		t.Fatal("tools/list must be deterministic")
	}
	for _, x := range result(t, r[0])["tools"].([]any) {
		tool := x.(map[string]any)
		if tool["outputSchema"] == nil || tool["inputSchema"].(map[string]any)["type"] != "object" || tool["annotations"] == nil {
			t.Errorf("tool %v incompletely described", tool["name"])
		}
	}
	if result(t, r[0])["ttlMs"] == nil || result(t, r[0])["cacheScope"] != "public" {
		t.Error("modern tools/list carries ttlMs and cacheScope")
	}
}

// TestStructuredContentConformsToTheOutputSchema checks the MUST of the spec
// for every tool that succeeds on the fake repository.
func TestStructuredContentConformsToTheOutputSchema(t *testing.T) {
	h := newHarness()
	h.log.Operations = []oplog.Operation{{ID: "op0", Kind: oplog.Deliver, Title: "x"}}
	schemas := map[string]map[string]any{}
	for _, x := range toolDefinitions() {
		schemas[x["name"].(string)] = x["outputSchema"].(map[string]any)
	}
	for i, call := range []struct{ name, args string }{
		{"ship_status", `{}`}, {"ship_plan", `{}`}, {"ship_log", `{}`}, {"ship_doctor", `{}`}, {"ship_deliver", `{}`},
		{"ship_config_get", `{"key":"ship.pullRequest.maxLines"}`},
		{"ship_config_explain", `{"key":"ship.issues.keyword"}`},
	} {
		name := call.name
		r := h.exchange(t, modern(i+1, "tools/call", fmt.Sprintf(`"name":%q,"arguments":%s`, name, call.args)))
		res := result(t, r[0])
		if res["isError"] != false {
			t.Fatalf("%s failed: %v", name, res["content"])
		}
		if err := schema.Validate(schemas[name], res["structuredContent"]); err != nil {
			t.Errorf("%s: structuredContent does not conform: %v", name, err)
		}
	}
}

func TestDeliverWithTheAgentTitle(t *testing.T) {
	h := newHarness()
	r := h.exchange(t, modern(1, "tools/call", `"name":"ship_deliver","arguments":{"title":"fix(app): stop the runner leak"}`))
	if res := result(t, r[0]); res["isError"] != false {
		t.Fatalf("deliver: %v", res["content"])
	}
	joined := strings.Join(*h.calls, ";")
	if !strings.Contains(joined, "switch fix/stop-runner-leak") || !strings.Contains(joined, "commit 🐛 fix(app): stop the runner leak") {
		t.Fatalf("calls = %q", *h.calls)
	}
	if h.log.Operations[0].AIAssistant != "host agent" {
		t.Error("an agent-written plan must be recorded for the AI-assisted mention")
	}
}

func TestDeliverDryRunChangesNothing(t *testing.T) {
	h := newHarness()
	r := h.exchange(t, modern(1, "tools/call", `"name":"ship_deliver","arguments":{"title":"fix(app): stop the runner leak","dryRun":true}`))
	res := result(t, r[0])
	if res["isError"] != false {
		t.Fatalf("deliver: %v", res["content"])
	}
	structured := res["structuredContent"].(map[string]any)
	planned, _ := structured["planned"].([]any)
	if structured["dry_run"] != true || len(planned) == 0 || structured["branch"] != "fix/stop-runner-leak" {
		t.Fatalf("result = %v", structured)
	}
	if len(*h.calls) != 0 || len(h.log.Operations) != 0 {
		t.Fatalf("a dry run changed something: %q, %v", *h.calls, h.log.Operations)
	}
}

func TestAHumanEditedTitleIsNotAIAssisted(t *testing.T) {
	h := newHarness()
	var out bytes.Buffer
	srv := &Server{Workspace: h.ws, Attended: true}
	in := modern(1, "tools/call", `"name":"ship_deliver","arguments":{"title":"fix(app): stop the runner leak"}`) + "\n"
	if err := srv.Serve(context.Background(), strings.NewReader(in), &out); err != nil || len(h.log.Operations) != 1 {
		t.Fatalf("deliver: %v\n%s", err, out.String())
	}
	if h.log.Operations[0].AIAssistant != "" {
		t.Error("in the extension a human edits the title: no AI-assisted mention")
	}
}

func TestPlanReasonsAreRendered(t *testing.T) {
	r := newHarness().exchange(t, modern(1, "tools/call", `"name":"ship_plan","arguments":{}`))
	var plan app.PlanView
	data, _ := json.Marshal(result(t, r[0])["structuredContent"])
	if err := json.Unmarshal(data, &plan); err != nil || len(plan.Reasons) == 0 {
		t.Fatalf("plan = %s (%v)", data, err)
	}
	for _, reason := range plan.Reasons {
		if reason.Text == "" || reason.Text == reason.Key {
			t.Errorf("reason %s is not rendered: %q", reason.Key, reason.Text)
		}
	}
}

func TestAttendedModeIsForHumans(t *testing.T) {
	published := func(attended bool) bool {
		h := newHarness()
		forge := &testkit.FakeForge{}
		git := testkit.HealthyRepository("feat/vat")
		git.CommitMessages = []string{"✨ feat: add vat"}
		h.ws.Git, h.ws.Forge = git, forge
		var out bytes.Buffer
		srv := &Server{Workspace: h.ws, Attended: attended}
		in := modern(1, "tools/call", `"name":"ship_publish","arguments":{}`) + "\n"
		if err := srv.Serve(context.Background(), strings.NewReader(in), &out); err != nil || len(forge.Created) != 1 {
			t.Fatalf("publish: %v\n%s", err, out.String())
		}
		return forge.Created[0].Draft
	}
	if !published(false) {
		t.Error("an agent's pull request must be a draft")
	}
	if published(true) {
		t.Error("in the extension, the human reviewed the plan: no forced draft")
	}
}

func TestToolErrorsAreForTheModel(t *testing.T) {
	h := newHarness()
	h.log.Operations = []oplog.Operation{{ID: "op1", Kind: oplog.Publish, PullRequest: 8, PullRequestCreated: true,
		After: oplog.RepositoryView{Branch: "feat/x"}}}
	r := h.exchange(t,
		modern(1, "tools/call", `"name":"ship_deliver","arguments":{"title":"Fixed stuff."}`),
		modern(2, "tools/call", `"name":"ship_undo","arguments":{}`),
		modern(3, "tools/call", `"name":"ship_plan","arguments":{"bogus":1}`),
	)
	for i, want := range []string{"SHIP-E0102", "SHIP-E0407", "invalid arguments"} {
		res := result(t, r[i])
		text := res["content"].([]any)[0].(map[string]any)["text"].(string)
		if res["isError"] != true || !strings.Contains(text, want) {
			t.Errorf("call %d: %v", i+1, res)
		}
	}
	if strings.Contains(strings.Join(*h.calls, ";"), "commit") {
		t.Error("an invalid title must commit nothing")
	}
}

// Without --attended an agent drives the workspace: it keeps its safeguards
// (the secret scan exempts only what the committed settings file says).
func TestAgentWorkspace(t *testing.T) {
	if ws := (&Server{}).workspace(true); !ws.Agent {
		t.Error("an unattended server is driven by an agent")
	}
	if ws := (&Server{Attended: true}).workspace(false); ws.Agent {
		t.Error("an attended server is driven by a human")
	}
}

// TestOperationsRecordTheAgent: #49. An operation made through the server
// records the agent: --client first, else the name of the initialize
// handshake; a human in the extension (attended) records none.
func TestOperationsRecordTheAgent(t *testing.T) {
	deliver := modern(2, "tools/call", `"name":"ship_deliver","arguments":{}`) + "\n"
	for _, tt := range []struct {
		name     string
		client   string
		attended bool
		in, want string
	}{
		{"--client", "codex", false, deliver, "codex"},
		{"handshake", "", false, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"Claude-Code","version":"2"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ship_deliver","arguments":{}}}` + "\n", "claude-code"},
		{"no name", "", false, deliver, "unknown"},
		{"a human", "vscode", true, deliver, ""},
	} {
		h := newHarness()
		srv := &Server{Workspace: h.ws, Client: tt.client, Attended: tt.attended}
		var out bytes.Buffer
		if err := srv.Serve(context.Background(), strings.NewReader(tt.in), &out); err != nil || len(h.log.Operations) != 1 {
			t.Fatalf("%s: %v, %d operations\n%s", tt.name, err, len(h.log.Operations), out.String())
		}
		if got := h.log.Operations[0].Agent; got != tt.want {
			t.Errorf("%s: agent %q, want %q", tt.name, got, tt.want)
		}
	}
}
