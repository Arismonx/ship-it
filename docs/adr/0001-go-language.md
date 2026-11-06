# ADR-0001: Go for the core

**Status:** accepted (2026-10-22, user choice)

## Context
We need a single binary, with no runtime, installable without admin rights by individuals, agencies and public administrations, with a minimum of dependencies. The author also wants to learn.

## Options
| | Pros | Cons |
|---|---|---|
| **Go** | Stdlib covering HTTP, TLS, JSON and exec; TLS verification through the Windows CryptoAPI (corporate CAs) ✅; trivial cross-compilation; the ecosystem of gh, gh-stack, lazygit | Frequent antivirus false positives (Go FAQ); verbose error handling; a rich TUI = a dependency (Bubble Tea) |
| Rust | Performance, safety, a rich learning experience; reqwest + rustls-platform-verifier (Windows store) | More crates; aws-lc-rs requires cmake + NASM to build on Windows; slow compilation |
| TypeScript/Bun | Continuity with the draft | Heavy binary; npm ecosystem (dependency surface) |
| Zig | Absolute minimalism | No git library, TLS and HTTP by hand: risk of getting bogged down |

## Decision
**Go** (current stable version; ❓ 1.27 if released).

## Consequences
- Windows signing is essential as soon as possible (see [compliance.md](../design/compliance.md)).
- A critical part could be rewritten in Rust later if a performance need is measured (not planned).
- The VS Code extension stays in TypeScript (imposed by VS Code), but without business logic.
