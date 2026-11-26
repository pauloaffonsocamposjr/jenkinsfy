# Development image for jenkinsfy. Tool versions match CI
# (.github/workflows/ci.yml): Go 1.27 and golangci-lint v2.15.0.
FROM mcr.microsoft.com/devcontainers/go:1.27-bookworm

# Replace the base image's golangci-lint with the exact binary CI uses.
COPY --from=docker.io/golangci/golangci-lint:v2.15.0 /usr/bin/golangci-lint /go/bin/golangci-lint

# podman runs the throwaway Jenkins used by `make e2e` inside this container
# (nested containers, so the devcontainer runs with --privileged).
# shellcheck covers the scripts under scripts/ and test/.
RUN apt-get update \
    && export DEBIAN_FRONTEND=noninteractive \
    && apt-get install -y --no-install-recommends \
        podman fuse-overlayfs slirp4netns uidmap shellcheck \
    && rm -rf /var/lib/apt/lists/*

# Inside a rootless container there are too few user IDs for a nested
# rootless podman, so the inner podman runs as root through sudo (the
# PODMAN variable in devcontainer.json). Storage and engine settings that
# work inside another container:
COPY containers.conf /etc/containers/containers.conf
COPY storage.conf /etc/containers/storage.conf
