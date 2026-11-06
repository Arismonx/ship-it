// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

import { existsSync } from "node:fs";
import { join } from "node:path";

/**
 * Finds the `ship` binary: the shipIt.path setting, else the one bundled in
 * the platform VSIX (bin/), else `ship` from the PATH (docs/design/vscode-extension.md). Nothing is
 * downloaded at run time.
 */
export function resolveBinary(setting: string, extensionPath: string, platform: NodeJS.Platform): string {
  if (setting.trim() !== "") {
    return setting.trim();
  }
  const bundled = join(extensionPath, "bin", platform === "win32" ? "ship.exe" : "ship");
  return existsSync(bundled) ? bundled : "ship";
}
