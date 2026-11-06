// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

import assert from "node:assert/strict";
import { PassThrough } from "node:stream";
import { test } from "node:test";
import { McpClient, protocolVersion } from "../mcpClient";

/** A fake `ship mcp`: answers each request line with reply(request). */
function fakeServer(reply: (request: any) => unknown) {
  const toServer = new PassThrough();
  const fromServer = new PassThrough();
  const sent: any[] = [];
  let buffer = "";
  toServer.setEncoding("utf8");
  toServer.on("data", (chunk: string) => {
    buffer += chunk;
    let i: number;
    while ((i = buffer.indexOf("\n")) >= 0) {
      const request = JSON.parse(buffer.slice(0, i));
      buffer = buffer.slice(i + 1);
      sent.push(request);
      const answer = reply(request);
      if (answer === undefined) {
        continue; // leave this request pending
      }
      // Answers split over two writes, as pipes may deliver them.
      const line = JSON.stringify({ jsonrpc: "2.0", id: request.id, ...(answer as object) }) + "\n";
      fromServer.write(line.slice(0, 5));
      fromServer.write(line.slice(5));
    }
  });
  return { client: new McpClient(toServer, fromServer, "9.9.9"), sent, fromServer };
}

test("tool calls carry the 2026-07-28 metadata and return structured content", async () => {
  const { client, sent } = fakeServer(() => ({ result: { resultType: "complete", isError: false, structuredContent: { uncommitted: 3 } } }));
  const outcome = await client.callTool("ship_status", { offline: true });
  assert.deepEqual(outcome, { ok: true, result: { uncommitted: 3 } });
  const meta = sent[0].params._meta;
  assert.equal(meta["io.modelcontextprotocol/protocolVersion"], protocolVersion);
  assert.deepEqual(meta["io.modelcontextprotocol/clientCapabilities"], {});
  assert.equal(sent[0].method, "tools/call");
  assert.deepEqual(sent[0].params.arguments, { offline: true });
});

test("tool errors become messages", async () => {
  const { client } = fakeServer(() => ({ result: { isError: true, content: [{ type: "text", text: "SHIP-E0901 secret" }] } }));
  assert.deepEqual(await client.callTool("ship_deliver"), { ok: false, message: "SHIP-E0901 secret" });
});

test("protocol errors reject, and a stopped server fails what is pending", async () => {
  const { client, fromServer } = fakeServer((request) =>
    request.params.name === "nope" ? { error: { code: -32602, message: "Unknown tool: nope" } } : undefined,
  );
  await assert.rejects(client.request("tools/call", { name: "nope" }), /Unknown tool: nope \(-32602\)/);
  const hanging = client.request("tools/list");
  fromServer.end();
  await assert.rejects(hanging, /ship mcp stopped/);
});
