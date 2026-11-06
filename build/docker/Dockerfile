# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 Grégoire Favreau
#
# Image of the ship CLI with git, for CI jobs that check pull request and
# commit titles and branch names (`ship check title`, `ship check branch`)
# against the repository's conventions. It does not release anything: ship
# has no release command yet. Published to ghcr.io/gregoiref/ship by
# GoReleaser (dockers_v2 in .goreleaser.yaml).
# GoReleaser builds the binary first and copies it into the build context as
# linux/<arch>/ship, so this file cannot be built with a plain `docker build .`:
# run `go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean --skip=sbom`
# (needs Docker with buildx, and QEMU for the arm64 RUN step).
#
# Base image: Alpine, the Docker Official Image. ship shells out to git, so
# distroless or scratch are not enough. Alpine was preferred to
# cgr.dev/chainguard/git (free tier: only "latest" tags, no shell, which CI
# runners such as GitLab need to run `script:` steps) and to debian-slim (a
# larger base for the same git). Git 2.54 in Alpine 3.24, above the 2.43 ship doctor wants.
# Tag 3.24.3, index digest checked on Docker Hub 2026-10-23; Dependabot (docker
# ecosystem) keeps it up to date.
FROM alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a

# git: ship's only runtime dependency. ca-certificates: TLS to the GitHub API.
# openssh-client: git remotes over SSH.
# The repository is mounted at /repo and usually belongs to another user than
# the container's: trust that one path (CVE-2022-24765 protects shared
# machines, not a single-user container), as actions/checkout does.
RUN apk add --no-cache git ca-certificates openssh-client \
 && git config --system --add safe.directory /repo \
 && addgroup -g 65532 ship \
 && adduser -D -u 65532 -G ship -h /home/ship ship

ARG TARGETPLATFORM
COPY --chmod=0755 $TARGETPLATFORM/ship /usr/local/bin/ship
# Licences of ship and of what its binary links (extra_files in
# .goreleaser.yaml), where Alpine packages put theirs: abuild's default_doc
# moves usr/share/licenses into the -doc subpackage, next to usr/share/doc.
COPY --chmod=0644 LICENSE THIRD_PARTY_NOTICES.txt /usr/share/licenses/ship/

# Numeric, so that Kubernetes runAsNonRoot can check it.
USER 65532:65532
ENV HOME=/home/ship
WORKDIR /repo
ENTRYPOINT ["/usr/local/bin/ship"]
CMD ["--help"]
