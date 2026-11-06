// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { McpClient } from "../mcpClient";

// The contract between this extension and the Go binary: the generated types
// must match what `ship mcp` really answers. Set SHIP_BIN to the binary.
const binary = process.env.SHIP_BIN;

test("the extension's client against the real ship mcp", { skip: !binary && "SHIP_BIN is not set" }, async () => {
  const repo = mkdtempSync(join(tmpdir(), "ship-it-contract-"));
  const git = (...args: string[]) => execFileSync("git", args, { cwd: repo });
  git("init", "--quiet", "--initial-branch=main");
  git("config", "user.name", "contract");
  git("config", "user.email", "contract@example.invalid");
  writeFileSync(join(repo, "README.md"), "# shop\n");
  git("add", "--all");
  git("commit", "--quiet", "-m", "🎉 chore: init");
  writeFileSync(join(repo, "vat.go"), "package shop\n");

  const server = spawn(binary!, ["mcp", "--attended"], { cwd: repo, stdio: ["pipe", "pipe", "inherit"] });
  try {
    const client = new McpClient(server.stdin!, server.stdout!, "contract");
    const plan = await client.callTool("ship_plan");
    assert.ok(plan.ok, plan.ok ? "" : plan.message);
    assert.equal(plan.result.title, "✨ feat: add vat");
    assert.equal(plan.result.branch, "feat/add-vat");
    assert.deepEqual(plan.result.files, ["vat.go"]);

    const status = await client.callTool("ship_status", { offline: true });
    assert.ok(status.ok);
    assert.equal(status.result.uncommitted, 1);
    assert.equal(status.result.next.key, "status.next.ship");
  } finally {
    server.stdin!.end();
  }
});
