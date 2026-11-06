# ADR-0003: GitHub authentication

**Status:** accepted for the resolution of existing tokens (implemented in [`internal/adapters/ghauth`](../../internal/adapters/ghauth/resolver.go) on 2026-10-23); `ship login`, the keychain and the credential helper remain proposed.

## Decision: reuse what the user already has
Resolution order; the first one found wins and is remembered per host for the lifetime of the process:
1. `SHIP_TOKEN` (CI, agents);
2. for github.com and `*.ghe.com`: `GH_TOKEN` then `GITHUB_TOKEN`; for a GitHub Enterprise Server: `GH_ENTERPRISE_TOKEN` then `GITHUB_ENTERPRISE_TOKEN`. These are `gh`'s variables and precedence (✅ V29). A github.com token is therefore **never sent** to another host;
3. `gh auth token --hostname <host>` if `gh` is installed (✅ V8), with `GH_PROMPT_DISABLED=1`;
4. *(coming)* the OS keychain filled by `ship login`;
5. **git's credential helpers** (`git credential fill`, ✅ V30): Git Credential Manager, osxkeychain, libsecret… For github.com, git has not accepted passwords since 2021, so the stored secret is a token the API can use. Run with `GIT_TERMINAL_PROMPT=0` and `GCM_INTERACTIVE=false` (✅ V31): **never a prompt or a window**.

Without a token, `ship` keeps the local commit and answers with exit code `2` ("human action required") and an exact remedy (`gh auth login` or `GH_TOKEN`), then `ship publish`.

The push goes through the user's git configuration (SSH key or credential helper), like a hand-typed `git push`: ship-it does not inject a token into git.

The token is a `ports.Token` type whose value is never displayed: `fmt` (`%v`, `%+v`, `%#v`) and `slog` show `[REDACTED]`, as a test proves.

## Still proposed
- `ship login` with the **OAuth device flow**, without a `client_secret`, to be enabled in the app settings. An OAuth App must be registered (its *client ID* is public).
- ship-it as a **git credential helper** for HTTPS, for a single git + API authentication.

## Open questions
- **OAuth App or GitHub App?** A GitHub App offers fine-grained permissions and expiring tokens (the device flow is supported), but installing it per organisation can be a blocker at client sites. Earlier decision: OAuth App.
- Scopes: `repo` + `read:org` + `write:ssh_signing_key` (systematic signing, N7); `workflow` requested on demand. ✅ GitHub docs.
- **Multiple profiles** tied to the remote's owner (N1): one token per profile in the keychain.
- Keychain without a dependency: Windows Credential Manager through syscalls, `security` on macOS, `secret-tool` on Linux (❓ to validate).
