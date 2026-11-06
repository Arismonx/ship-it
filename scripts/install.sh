#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Grégoire Favreau
#
# Installs ship from a GitHub release of GregoireF/ship-it, on Linux and macOS,
# into a user directory (no sudo):
#
#   curl -fsSL https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/GregoireF/ship-it/main/scripts/install.sh | sh -s -- --version 0.1.0
#
# It downloads the archive for this system and checksums.txt, checks the
# archive's SHA-256 against checksums.txt and stops on any mismatch. It also
# checks where the release comes from, with the first verifier it finds:
# - cosign: checksums.txt was signed by the release workflow of
#   GregoireF/ship-it for that tag (keyless, Sigstore);
# - otherwise the GitHub CLI, logged in: `gh attestation verify` checks the
#   archive's build provenance, made by that same workflow for that tag.
# These are the checks of docs/design/workflows.md §5. Without either, it
# checks the SHA-256 only, unless --require-signature is given. Nothing
# downloaded is run or installed before these checks pass.
#
# Options (or environment variables; a flag variable counts only when it is
# 1 or true):
#   --version X.Y.Z       SHIP_VERSION                release to install (default: latest)
#   --dir DIR             SHIP_INSTALL_DIR            where to put ship (default: ~/.local/bin)
#   --no-cosign           SHIP_NO_COSIGN=1            do not use cosign even if it is there
#   --require-signature   SHIP_REQUIRE_SIGNATURE=1    fail when neither cosign nor gh can check
# For tests and mirrors only: SHIP_INSTALL_BASE_URL replaces
# https://github.com/GregoireF/ship-it/releases/download/<tag> (it needs a
# version; curl also accepts file:// there).
#
# Everything runs from main, called on the last line: a download cut short
# defines functions at most and runs nothing.

set -eu

repo="GregoireF/ship-it"

say() { printf '%s\n' "$*"; }
die() {
  printf 'ship install: %s\n' "$*" >&2
  exit 1
}

usage() {
  say "Usage: install.sh [--version X.Y.Z] [--dir DIR] [--no-cosign] [--require-signature]"
  say "Installs ship from https://github.com/$repo/releases (default: latest, into ~/.local/bin)."
}

# True when $1 is 1 or true: SHIP_NO_COSIGN=0 or =false must not turn the
# check off.
is_true() {
  case "$1" in
    1 | true) return 0 ;;
    *) return 1 ;;
  esac
}

have() { command -v "$1" >/dev/null 2>&1; }

# Downloads $1 to the file $2; fails on any HTTP error. GNU wget applies
# --https-only to the links of a recursive download only; the URLs given
# here are https in the first place.
fetch() {
  if have curl; then
    curl --proto '=https,file' --tlsv1.2 -fsSL -o "$2" "$1"
  elif have wget; then
    wget --https-only --secure-protocol=TLSv1_2 -q -O "$2" "$1"
  else
    die "curl or wget is needed"
  fi
}

# Prints the tag of the latest release, read from the redirect of
# /releases/latest (no API call, so no API rate limit).
latest_tag() {
  url="https://github.com/$repo/releases/latest"
  if have curl; then
    final=$(curl --proto '=https' --tlsv1.2 -fsSLI -o /dev/null -w '%{url_effective}' "$url") || final=""
  elif have wget; then
    final=$(wget --https-only --secure-protocol=TLSv1_2 -q -S --spider "$url" 2>&1 |
      sed -n 's/^ *[Ll]ocation: *//p' | tail -n 1 | tr -d '\r') || final=""
  else
    die "curl or wget is needed"
  fi
  case "$final" in
    */releases/tag/*) printf '%s\n' "${final##*/}" ;;
    *) die "could not find the latest release of $repo" ;;
  esac
}

sha256_of() {
  if have sha256sum; then
    sha256sum "$1" | cut -d ' ' -f 1
  elif have shasum; then
    shasum -a 256 "$1" | cut -d ' ' -f 1
  elif have openssl; then
    openssl dgst -sha256 "$1" | sed 's/^.*= *//'
  else
    die "sha256sum, shasum or openssl is needed to check the download"
  fi
}

main() {
  version="${SHIP_VERSION:-}"
  install_dir="${SHIP_INSTALL_DIR:-${HOME:?HOME is not set}/.local/bin}"
  no_cosign=""
  is_true "${SHIP_NO_COSIGN:-}" && no_cosign=1
  require_signature=""
  is_true "${SHIP_REQUIRE_SIGNATURE:-}" && require_signature=1
  base_url="${SHIP_INSTALL_BASE_URL:-}"

  while [ $# -gt 0 ]; do
    case "$1" in
      --version)
        [ $# -ge 2 ] || die "--version needs a value"
        version="$2"
        shift 2
        ;;
      --version=*)
        version="${1#*=}"
        shift
        ;;
      --dir)
        [ $# -ge 2 ] || die "--dir needs a value"
        install_dir="$2"
        shift 2
        ;;
      --dir=*)
        install_dir="${1#*=}"
        shift
        ;;
      --no-cosign)
        no_cosign=1
        shift
        ;;
      --require-signature)
        require_signature=1
        shift
        ;;
      -h | --help)
        usage
        exit 0
        ;;
      *) die "unknown option: $1 (see --help)" ;;
    esac
  done

  case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "unsupported system $(uname -s): see https://github.com/$repo#install" ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "unsupported architecture $(uname -m): see https://github.com/$repo#install" ;;
  esac
  # Rosetta 2: an x86_64 shell on Apple silicon still gets the native binary.
  if [ "$os" = darwin ] && [ "$arch" = amd64 ] &&
    [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
    arch=arm64
  fi

  if [ -z "$version" ]; then
    [ -z "$base_url" ] || die "SHIP_INSTALL_BASE_URL needs a version"
    tag=$(latest_tag)
  else
    tag="v${version#v}"
  fi
  # The tag goes into URLs and file names: semver characters only.
  case "$tag" in
    v[0-9]*) ;;
    *) die "not a version: $tag" ;;
  esac
  case "${tag#v}" in
    *[!0-9A-Za-z.+-]*) die "not a version: $tag" ;;
  esac
  version="${tag#v}"
  base_url="${base_url:-https://github.com/$repo/releases/download/$tag}"
  archive="ship_${version}_${os}_${arch}.tar.gz"

  # Which verifier checks the origin of the release: cosign, else a logged-in
  # gh, else none.
  verifier=""
  if [ -z "$no_cosign" ] && have cosign; then
    verifier=cosign
  elif have gh && gh auth status >/dev/null 2>&1; then
    verifier=gh
  elif [ -n "$require_signature" ]; then
    die "--require-signature: install cosign, or the GitHub CLI (gh) and run gh auth login"
  fi

  tmp=$(mktemp -d 2>/dev/null || mktemp -d -t ship-install)
  trap 'rm -rf "$tmp"' EXIT
  trap 'exit 1' HUP INT TERM

  say "Downloading ship $version for $os/$arch"
  fetch "$base_url/$archive" "$tmp/$archive" || die "could not download $base_url/$archive"
  fetch "$base_url/checksums.txt" "$tmp/checksums.txt" || die "could not download $base_url/checksums.txt"

  if [ "$verifier" = cosign ]; then
    fetch "$base_url/checksums.txt.sigstore.json" "$tmp/checksums.txt.sigstore.json" ||
      die "could not download $base_url/checksums.txt.sigstore.json"
    cosign verify-blob "$tmp/checksums.txt" \
      --bundle "$tmp/checksums.txt.sigstore.json" \
      --certificate-identity "https://github.com/$repo/.github/workflows/release.yml@refs/tags/$tag" \
      --certificate-oidc-issuer https://token.actions.githubusercontent.com ||
      die "checksums.txt is not signed by the release workflow of $repo for $tag"
    say "Signature of checksums.txt verified with cosign"
  fi

  # Exactly one line "<sha256>  <archive>" must name the archive.
  expected=$(awk -v f="$archive" '$2 == f || $2 == ("*" f) { print $1 }' "$tmp/checksums.txt")
  [ -n "$expected" ] || die "$archive is not listed in checksums.txt"
  [ "$(printf '%s\n' "$expected" | wc -l | tr -d ' ')" = 1 ] || die "$archive is listed more than once in checksums.txt"
  actual=$(sha256_of "$tmp/$archive")
  [ "$actual" = "$expected" ] || die "SHA-256 mismatch for $archive: expected $expected, got $actual"
  say "SHA-256 verified"

  if [ "$verifier" = gh ]; then
    gh attestation verify "$tmp/$archive" -R "$repo" \
      --signer-workflow "github.com/$repo/.github/workflows/release.yml" \
      --source-ref "refs/tags/$tag" >/dev/null ||
      die "$archive has no build provenance from the release workflow of $repo for $tag"
    say "Build provenance of $archive verified with gh"
  elif [ -z "$verifier" ]; then
    say "Neither cosign nor a logged-in gh found: checked the SHA-256 only"
    say "(install one of them to also check where the release comes from)"
  fi

  mkdir "$tmp/x"
  tar -xzf "$tmp/$archive" -C "$tmp/x" ship || die "could not extract ship from $archive"
  [ -f "$tmp/x/ship" ] || die "$archive holds no ship binary"
  mkdir -p "$install_dir"
  # Copy, then rename over the old binary in one step.
  cp "$tmp/x/ship" "$install_dir/.ship.new"
  chmod 755 "$install_dir/.ship.new"
  mv -f "$install_dir/.ship.new" "$install_dir/ship"
  say "Installed $install_dir/ship"

  case ":${PATH:-}:" in
    *":$install_dir:"*) say "Run: ship doctor" ;;
    *)
      say ""
      say "$install_dir is not on your PATH. Add this line to your shell profile"
      say "(~/.profile, ~/.bashrc or ~/.zshrc), then open a new terminal:"
      say ""
      say "  export PATH=\"$install_dir:\$PATH\""
      say ""
      say "Then run: ship doctor"
      ;;
  esac
}

main "$@"
