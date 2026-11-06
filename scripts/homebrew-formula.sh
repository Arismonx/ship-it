#!/bin/sh
# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Grégoire Favreau
#
# Prints the Homebrew formula of ship (Formula/ship.rb in the tap
# GregoireF/homebrew-tap) for one release, from that release's checksums.txt:
#
#   scripts/homebrew-formula.sh 1.2.0 dist/checksums.txt > ship.rb
#
# Run by .github/workflows/release.yml after GoReleaser, which then pushes
# the result to the tap (docs/design/workflows.md §5, "The tap deploy key").
# A formula and not a cask: Homebrew does not quarantine what a formula
# installs, so the unsigned macOS binaries need no workaround, and a formula
# also serves Homebrew on Linux. GoReleaser's own formula generator (`brews`)
# is deprecated and makes `goreleaser check` fail, hence this script.
# Reference: https://docs.brew.sh/Formula-Cookbook

set -eu

repo="GregoireF/ship-it"

die() {
  printf 'homebrew-formula: %s\n' "$*" >&2
  exit 1
}

main() {
  [ $# -eq 2 ] || die "usage: homebrew-formula.sh VERSION CHECKSUMS"
  version="${1#v}"
  sums="$2"
  # The version goes into URLs and Ruby strings: semver characters only.
  case "$version" in
    [0-9]*) ;;
    *) die "not a version: $1" ;;
  esac
  case "$version" in
    *[!0-9A-Za-z.+-]*) die "not a version: $1" ;;
  esac
  [ -r "$sums" ] || die "cannot read $sums"

  base="https://github.com/$repo/releases/download/v$version"
  # Assignments, not substitutions inside the here-document: a failing
  # target then stops the script (set -e) instead of printing a hole.
  darwin_amd64=$(target darwin amd64)
  darwin_arm64=$(target darwin arm64)
  linux_amd64=$(target linux amd64)
  linux_arm64=$(target linux arm64)

  cat <<EOF
# typed: false
# frozen_string_literal: true

# Written by the release workflow of $repo (scripts/homebrew-formula.sh);
# do not edit by hand, the next release replaces it.
class Ship < Formula
  desc "You write the code, ship handles git and GitHub"
  homepage "https://github.com/$repo"
  version "$version"
  license "Apache-2.0"

  on_macos do
    on_intel do
$darwin_amd64
    end
    on_arm do
$darwin_arm64
    end
  end

  on_linux do
    on_intel do
$linux_amd64
    end
    on_arm do
$linux_arm64
    end
  end

  def install
    bin.install "ship"
  end

  def caveats
    <<~EOS
      ship needs git 2.43 or later. Run \`ship doctor\` to check your setup.
    EOS
  end

  test do
    system "#{bin}/ship", "version"
  end
end
EOF
}

# Prints the url and sha256 lines of the archive for $1/$2, or fails when
# checksums.txt does not list it exactly once with a SHA-256.
target() {
  archive="ship_${version}_$1_$2.tar.gz"
  sum=$(awk -v f="$archive" '$2 == f || $2 == ("*" f) { print $1 }' "$sums")
  [ "$(printf '%s\n' "$sum" | wc -l | tr -d ' ')" = 1 ] && [ -n "$sum" ] ||
    die "$archive must be listed once in $sums"
  case "$sum" in
    *[!0-9a-f]*) die "not a SHA-256 for $archive: $sum" ;;
  esac
  [ ${#sum} -eq 64 ] || die "not a SHA-256 for $archive: $sum"
  printf '      url "%s/%s"\n' "$base" "$archive"
  printf '      sha256 "%s"\n' "$sum"
}

main "$@"
