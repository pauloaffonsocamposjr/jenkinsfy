# Changelog

All notable changes to jenkinsfy are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The public interface covered by SemVer is the command line (flags, exit codes)
and the shape of the generated Jenkinsfile. Packages under `internal/` are not
part of it.

## [Unreleased]

### Added

- Devcontainer (`.devcontainer/`) with the same tool versions as CI: Go 1.27,
  golangci-lint v2.15.0, shellcheck v0.11.0, make, gh, and podman so that
  `make e2e` runs Jenkins inside it. Set up for podman; see
  `.devcontainer/README.md` for Docker.
- `PODMAN` setting for `test/jenkins/verify.sh` (for example `sudo podman`).

### Changed

- `make lint` and the CI Lint job run a pinned, checksum-verified shellcheck
  (`scripts/install-shellcheck.sh`) on the repository's scripts and on the
  workflow's `run:` scripts. The CI Lint job also checks `CHANGELOG.md` now;
  before, both checks ran only in a local `make lint`.

### Fixed

- Release tags made by `make release` now have a `jenkinsfy vX.Y.Z` subject
  line and keep the changelog's `###` headings. Before, git stripped the
  headings as comments and the first bullet became the tag's title on GitHub.
- The release job's version check no longer runs a glob as a command name
  (shellcheck SC2211).

## [0.1.0] - 2026-11-26

### Added

- `jenkinsfy` command that converts a GitHub Actions workflow into a
  declarative Jenkinsfile, reading a file or stdin and writing stdout or `-o`.
- `-label FROM=TO`, `-strict`, `-q` and `-version` flags.
- Conversion of jobs (ordered by `needs`, independent jobs in `parallel`),
  `runs-on`, `container`, `strategy.matrix` with `exclude`, `env`, secrets,
  shells, `working-directory`, `timeout-minutes`, `continue-on-error`, job
  and step `if:`, `always()`/`failure()`/`cancelled()` post conditions,
  `actions/checkout`, `upload-artifact`/`download-artifact`, `docker://`
  actions, schedules, `workflow_dispatch` inputs and `concurrency`.
- Translation of `${{ }}` expressions to Groovy, including the `github`,
  `inputs`, `matrix`, `secrets`, `env`, `vars` and `runner` contexts, the
  status functions, `contains`/`startsWith`/`endsWith`/`format`/`join`/
  `toJSON`/`fromJSON`, and `&&`/`||` used as values.
- Warnings and `TODO(jenkinsfy)` comments for anything not converted exactly.
- CI/CD workflow (lint, test matrix with coverage gate, cross-compiled
  binaries, Jenkins end-to-end check, tag-driven releases), and the
  repository `Jenkinsfile` generated from it.

[Unreleased]: https://github.com/pauloaffonsocamposjr/jenkinsfy/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/pauloaffonsocamposjr/jenkinsfy/releases/tag/v0.1.0
