# Devcontainer

A development environment for jenkinsfy with the same tool versions as CI:

| Tool | Version | Notes |
| --- | --- | --- |
| Go | 1.27 | from `mcr.microsoft.com/devcontainers/go:1.27-bookworm` |
| golangci-lint | v2.15.0 | copied from the `golangci/golangci-lint:v2.15.0` image CI uses |
| make, git, gopls | from the base image | |
| podman | 4.3 (Debian) | runs the Jenkins used by `make e2e` |
| shellcheck | v0.11.0 | pinned and checksum-verified by `scripts/install-shellcheck.sh`, the same as CI |
| gh | latest | GitHub CLI devcontainer feature |

## Open it

**VS Code with podman** (the default setup):

1. Install the *Dev Containers* extension.
2. Set `"dev.containers.dockerPath": "podman"` in your VS Code user settings.
3. Run **Dev Containers: Reopen in Container**.

**VS Code with Docker:** remove `"--userns=keep-id"` from `runArgs` in
`devcontainer.json` (it is podman-only). Everything else is the same.

## Use it

```sh
make lint    # same checks as the CI Lint job
make cover   # tests with -race and the 80% coverage gate
make e2e     # Jenkins in a nested container; port 8080 is forwarded
make dist    # release binaries into dist/
```

## How it works

- **Nested containers.** `make e2e` starts Jenkins with podman inside this
  container, so the container runs with `--privileged`. A rootless container
  has too few user IDs for a nested rootless podman, so the inner podman runs
  as root through `sudo`. `PODMAN=sudo podman` tells
  `test/jenkins/verify.sh` to use it. Its images are kept in the
  `jenkinsfy-devcontainer-podman` volume, so rebuilding the devcontainer
  doesn't download Jenkins again.
- **File ownership.** With rootless podman, `--userns=keep-id` maps your host
  UID to the `vscode` user, so files created in the workspace stay owned by
  you on the host.
- `containers.conf` and `storage.conf` configure the inner podman to share
  the container's namespaces and to use `fuse-overlayfs`.
