# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Grégoire Favreau
#
# Dev container of ship-it contributors (CONTRIBUTING.md), also used by
# GitHub Codespaces. The image is the Microsoft Go dev container
# (https://github.com/devcontainers/images/tree/main/src/go): Go 1.27,
# Debian trixie, git from the devcontainers git feature (newer than the
# git 2.43 that `ship doctor` requires), nvm and the non-root user `vscode`.
#
# It lives in a Dockerfile rather than in devcontainer.json "image" so that
# Dependabot (docker ecosystem, .github/dependabot.yml) keeps the digest fresh.
# Tag 2.3.3-1.27-trixie, index digest checked on mcr.microsoft.com 2026-10-23.
FROM mcr.microsoft.com/devcontainers/go:2.3.3-1.27-trixie@sha256:8f1c83128e4bce9d745b1dc0f28f8c027e303d3886e2434966ff135296c41770
