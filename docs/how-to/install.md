# Install, update and remove ship-it

ship-it is one binary, `ship`. Pick the way you install your other tools.

## Install

| Where | Command | Puts `ship` in |
|---|---|---|
| Linux, macOS | `curl -fsSL https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.sh \| sh` | `~/.local/bin` |
| Windows (PowerShell) | `irm https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.ps1 \| iex` | `%LOCALAPPDATA%\Programs\ship` |
| Homebrew (macOS, Linux) | `brew install gregoiref/tap/ship` | Homebrew's prefix |
| Scoop (Windows) | `scoop bucket add gregoiref https://github.com/GregoireF/homebrew-tap` then `scoop install ship` | Scoop's apps |
| Go 1.27 | `go install github.com/GregoireF/ship-it/cmd/ship@latest` | `$(go env GOPATH)/bin` |
| Container | `docker run --rm -v "$PWD:/repo" ghcr.io/gregoiref/ship:latest plan` | nothing installed |

Or download the archive for your system from the [latest release](https://github.com/GregoireF/ship-it/releases/latest).

You need git 2.43 or later. If you already use the GitHub CLI or push over HTTPS, ship finds your GitHub token by itself; [GitHub access](../reference/github-access.md) lists where it looks and the permissions it needs. Then run `ship doctor`: it says what is missing, with the command that fixes it.

### What the install scripts check

Read them first if you prefer: [install.sh](../../scripts/install.sh), [install.ps1](../../scripts/install.ps1).

- The archive is checked against the release's SHA-256 checksums; a mismatch stops the script.
- If cosign is installed, the scripts also check that the checksums file was signed by this repository's release workflow. Without cosign, a logged-in GitHub CLI checks the archive's build provenance instead.
- `--require-signature` (`-RequireSignature` on Windows) stops when neither check is possible.
- `SHIP_VERSION` installs a given version, `SHIP_INSTALL_DIR` another folder.
- When the folder is not on your `PATH`, they print the line to add to your shell profile.

To check a release by hand: `gh attestation verify ship_<version>_linux_amd64.tar.gz -R GregoireF/ship-it`, or for the image `gh attestation verify oci://ghcr.io/gregoiref/ship:<version> -R GregoireF/ship-it`.

## Update

Run the same command again: the scripts install the latest release over the old one. With Homebrew, `brew upgrade ship`; with Scoop, `scoop update ship`; with Go, `go install …@latest` again. `ship version` tells you which one you have.

## Remove

Do it in this order, while `ship` is still there:

1. In each repository where you used it, run `ship uninstall`. It lists what it would remove, then asks: `.git/ship` (the checkpoints, the operation log and the run logs), and the `ship-it` entry that `ship mcp install` wrote in a coding agent's project file. The agent's other servers stay; a file that only held ship-it goes. **The checkpoints are your way back**: after this, `ship undo` has nothing left to undo. `--dry-run` only lists.
2. Run `ship uninstall --user` once, for your own files:

   | | Settings | Cache | Logs |
   |---|---|---|---|
   | Linux | `~/.config/ship-it` | `~/.cache/ship-it` | `~/.local/state/ship-it` |
   | macOS | `~/Library/Application Support/ship-it` | `~/Library/Caches/ship-it` | `~/Library/Logs/ship-it` |
   | Windows | `%APPDATA%\ship-it` | `%LOCALAPPDATA%\ship-it\cache` | `%LOCALAPPDATA%\ship-it\logs` |

   (`$XDG_CONFIG_HOME`, `$XDG_CACHE_HOME` and `$XDG_STATE_HOME` move the Linux folders when they are set.) An organisation policy deployed by your IT department is never touched.
3. Remove the binary the way you installed it: `brew uninstall ship`, `scoop uninstall ship`, or delete `ship` from `~/.local/bin`, `%LOCALAPPDATA%\Programs\ship` or `$(go env GOPATH)/bin`. The container image goes with `docker image rm ghcr.io/gregoiref/ship`.

Already removed the binary? The folders above and `.git/ship` can be deleted by hand.
