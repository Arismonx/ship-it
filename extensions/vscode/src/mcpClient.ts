// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

import type { Readable, Writable } from "node:stream";
import type { ToolResults } from "./generated/tools";

/** The MCP revision spoken: stateless, metadata on every request. */
export const protocolVersion = "2026-07-28";

/** A tool call either succeeds with its typed result, or explains why not. */
export type ToolOutcome<T> = { ok: true; result: T } | { ok: false; message: string };

interface Pending {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
}

/**
 * A client of `ship mcp` over a pair of streams: one JSON-RPC message per
 * line (MCP stdio transport). It holds no logic of ship-it: it sends tool
 * calls and returns what the binary answers.
 */
export class McpClient {
  private nextId = 1;
  private readonly pending = new Map<number, Pending>();
  private buffer = "";

  constructor(
    private readonly input: Writable,
    output: Readable,
    private readonly clientVersion: string,
  ) {
    output.setEncoding("utf8");
    output.on("data", (chunk: string) => this.receive(chunk));
    output.on("end", () => this.failAll(new Error("ship mcp stopped")));
  }

  /** Calls a tool and returns its structured result. */
  async callTool<K extends keyof ToolResults>(name: K, args: Record<string, unknown> = {}): Promise<ToolOutcome<ToolResults[K]>> {
    const result = (await this.request("tools/call", { name, arguments: args })) as {
      isError?: boolean;
      structuredContent?: ToolResults[K];
      content?: { type: string; text?: string }[];
    };
    if (result.isError) {
      const message = (result.content ?? []).map((c) => c.text ?? "").join("\n");
      return { ok: false, message };
    }
    return { ok: true, result: result.structuredContent as ToolResults[K] };
  }

  /** Sends one request with the per-request metadata of 2026-07-28. */
  request(method: string, params: Record<string, unknown> = {}): Promise<unknown> {
    const id = this.nextId++;
    const message = {
      jsonrpc: "2.0",
      id,
      method,
      params: {
        ...params,
        _meta: {
          "io.modelcontextprotocol/protocolVersion": protocolVersion,
          "io.modelcontextprotocol/clientCapabilities": {},
          "io.modelcontextprotocol/clientInfo": { name: "ship-it-vscode", version: this.clientVersion },
        },
      },
    };
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.input.write(JSON.stringify(message) + "\n");
    });
  }

  private receive(chunk: string): void {
    this.buffer += chunk;
    let newline: number;
    while ((newline = this.buffer.indexOf("\n")) >= 0) {
      const line = this.buffer.slice(0, newline).trim();
      this.buffer = this.buffer.slice(newline + 1);
      if (line !== "") {
        this.dispatch(line);
      }
    }
  }

  private dispatch(line: string): void {
    let message: { id?: number; result?: unknown; error?: { code: number; message: string } };
    try {
      message = JSON.parse(line);
    } catch {
      return; // ship mcp writes only JSON-RPC on stdout; ignore anything else
    }
    if (typeof message.id !== "number") {
      return; // notifications: nothing to settle
    }
    const pending = this.pending.get(message.id);
    if (!pending) {
      return;
    }
    this.pending.delete(message.id);
    if (message.error) {
      pending.reject(new Error(`${message.error.message} (${message.error.code})`));
    } else {
      pending.resolve(message.result);
    }
  }

  private failAll(error: Error): void {
    for (const pending of this.pending.values()) {
      pending.reject(error);
    }
    this.pending.clear();
  }
}
