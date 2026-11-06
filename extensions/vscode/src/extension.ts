// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

import { spawn, type ChildProcess } from "node:child_process";
import * as vscode from "vscode";
import { resolveBinary } from "./binary";
import { McpClient } from "./mcpClient";

/** How often the status bar refreshes without file events. */
const refreshInterval = 60_000;

let server: ChildProcess | undefined;
let client: McpClient | undefined;

export function activate(context: vscode.ExtensionContext): void {
  const version = String(context.extension.packageJSON.version ?? "0.0.0");
  const binary = () =>
    resolveBinary(vscode.workspace.getConfiguration("shipIt").get<string>("path", ""), context.extensionPath, process.platform);

  // VS Code agents (Copilot agent mode…) get `ship mcp`: the agent safeguards
  // of ADR-0010 apply to them (drafts, no remote undo).
  context.subscriptions.push(
    vscode.lm.registerMcpServerDefinitionProvider("ship-it.mcp", {
      provideMcpServerDefinitions: () => [new vscode.McpStdioServerDefinition("ship-it", binary(), ["mcp"], {}, version)],
    }),
  );

  // The extension itself is a human interface: `ship mcp --attended`.
  const connect = (): McpClient | undefined => {
    if (client) {
      return client;
    }
    const root = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
    if (!root) {
      return undefined;
    }
    server = spawn(binary(), ["mcp", "--attended"], { cwd: root, stdio: ["pipe", "pipe", "pipe"] });
    server.on("exit", () => {
      client = undefined;
      server = undefined;
    });
    server.on("error", (error) => {
      void vscode.window.showErrorMessage(`ship-it: cannot start ${binary()} (${error.message}). Set shipIt.path or install ship.`);
    });
    client = new McpClient(server.stdin!, server.stdout!, version);
    return client;
  };

  const status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
  status.command = "shipIt.ship";
  context.subscriptions.push(status);
  const refresh = async () => {
    const outcome = await connect()?.callTool("ship_status", { offline: true }).catch(() => undefined);
    if (!outcome?.ok) {
      status.hide();
      return;
    }
    const files = outcome.result.uncommitted;
    status.text = files > 0 ? `$(rocket) Ship ${files}` : "$(rocket) Ship";
    status.tooltip = `${outcome.result.branch ?? ""}: ${files} file(s) not shipped`;
    status.show();
  };
  void refresh();
  const timer = setInterval(() => void refresh(), refreshInterval);
  context.subscriptions.push({ dispose: () => clearInterval(timer) });
  context.subscriptions.push(vscode.workspace.onDidSaveTextDocument(() => void refresh()));

  context.subscriptions.push(
    vscode.commands.registerCommand("shipIt.ship", async () => {
      const mcp = connect();
      if (!mcp) {
        return;
      }
      const plan = await mcp.callTool("ship_plan");
      if (!plan.ok) {
        void vscode.window.showWarningMessage(plan.message);
        return;
      }
      const { title, branch, files, secrets } = plan.result;
      if (secrets && secrets.length > 0) {
        void vscode.window.showErrorMessage(
          `ship-it: ${secrets.length} secret(s) in your changes, nothing shipped: ` +
            secrets.map((s) => `${s.path}:${s.line}`).join(", "),
        );
        return;
      }
      // The review: one confirmation, as in the terminal (Enter = ship).
      const choice = await vscode.window.showInformationMessage(
        title,
        { modal: true, detail: `Branch ${branch}\n${(files ?? []).length} file(s): ${(files ?? []).join(", ")}` },
        "Ship",
      );
      if (choice !== "Ship") {
        return;
      }
      await vscode.window.withProgress({ location: vscode.ProgressLocation.Notification, title: "ship-it" }, async (progress) => {
        progress.report({ message: "Committing…" });
        const delivered = await mcp.callTool("ship_deliver");
        if (!delivered.ok) {
          void vscode.window.showErrorMessage(delivered.message);
          return;
        }
        progress.report({ message: "Checks, push and pull request…" });
        const published = await mcp.callTool("ship_publish");
        if (!published.ok) {
          void vscode.window.showWarningMessage(`Committed locally. ${published.message}`);
          return;
        }
        const url = published.result.pull_request.url;
        const open = await vscode.window.showInformationMessage(`Pull request #${published.result.pull_request.number}`, "Open");
        if (open === "Open") {
          void vscode.env.openExternal(vscode.Uri.parse(url));
        }
      });
      void refresh();
    }),
    vscode.commands.registerCommand("shipIt.undo", async () => {
      const mcp = connect();
      if (!mcp) {
        return;
      }
      let outcome = await mcp.callTool("ship_undo");
      if (!outcome.ok && outcome.message.startsWith("SHIP-E0407")) {
        // Undoing a publication changes GitHub: the human confirms here.
        const choice = await vscode.window.showWarningMessage(outcome.message, { modal: true }, "Close it");
        if (choice !== "Close it") {
          return;
        }
        outcome = await mcp.callTool("ship_undo", { allowRemote: true });
      }
      if (outcome.ok) {
        void vscode.window.showInformationMessage("ship-it: undone.");
      } else {
        void vscode.window.showWarningMessage(outcome.message);
      }
      void refresh();
    }),
    vscode.commands.registerCommand("shipIt.status", async () => {
      const outcome = await connect()?.callTool("ship_status");
      if (outcome?.ok) {
        const s = outcome.result;
        void vscode.window.showInformationMessage(
          `${s.branch ?? "detached"}: ${s.uncommitted} file(s) not shipped` + (s.pull_request ? `, pull request #${s.pull_request.number}` : ""),
        );
      } else if (outcome) {
        void vscode.window.showWarningMessage(outcome.message);
      }
    }),
  );
}

export function deactivate(): void {
  // Closing stdin is the graceful shutdown of an MCP stdio server.
  server?.stdin?.end();
}
