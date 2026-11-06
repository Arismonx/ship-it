// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Grégoire Favreau

import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { resolveBinary } from "../binary";

test("the setting wins, then the bundled binary, then the PATH", () => {
  const extension = mkdtempSync(join(tmpdir(), "ship-it-ext-"));
  assert.equal(resolveBinary("", extension, "linux"), "ship");
  mkdirSync(join(extension, "bin"));
  writeFileSync(join(extension, "bin", "ship.exe"), "");
  assert.equal(resolveBinary("", extension, "win32"), join(extension, "bin", "ship.exe"));
  assert.equal(resolveBinary("", extension, "linux"), "ship");
  assert.equal(resolveBinary(" /opt/ship ", extension, "win32"), "/opt/ship");
});
