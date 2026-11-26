# Changelog

All notable changes to jenkinsfy are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
The public interface covered by SemVer is the command line (flags, exit codes)
and the shape of the generated Jenkinsfile. Packages under `internal/` are not
part of it.

## [Unreleased]

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

[Unreleased]: https://github.com/pauloaffonsocamposjr/jenkinsfy/compare/a959b1b...HEAD
