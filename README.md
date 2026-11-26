# jenkinsfy

GitHub Workflow to Jenkins converter: turns a GitHub Actions workflow
(`.github/workflows/*.yml`) into a declarative `Jenkinsfile`.

## Install

```sh
go install github.com/pauloaffonsocamposjr/jenkinsfy@latest
```

## Usage

```sh
jenkinsfy .github/workflows/ci.yml > Jenkinsfile
jenkinsfy -o Jenkinsfile -label ubuntu-latest=linux -label windows-latest=windows ci.yml
cat ci.yml | jenkinsfy
```

| Flag | Meaning |
| --- | --- |
| `-o file` | Write the Jenkinsfile to `file` instead of stdout. |
| `-label FROM=TO` | Map a GitHub runner label to a Jenkins agent label (repeatable). |
| `-strict` | Exit with status 2 when the conversion produced warnings. |
| `-q` | Don't print warnings. |

Anything that can't be converted exactly is reported as a warning on stderr
and marked in the output with a `// TODO(jenkinsfy):` comment.

## How it maps

| GitHub Actions | Jenkinsfile |
| --- | --- |
| job | `stage`. Jobs are ordered by `needs`, and independent jobs go in a `parallel` block |
| `runs-on` | `agent { label '...' }`. Several labels are joined with `&&` |
| `container` | `agent { docker { image ...; args ... } }` |
| `strategy.matrix` (+ `exclude`) | `matrix { axes { ... } excludes { ... } }` |
| `env` (workflow / job / step) | `environment {}` / stage `environment {}` / `withEnv` |
| `${{ secrets.X }}` | `credentials('X')` or `withCredentials([string(...)])` |
| `run` + `shell` | `sh`, `bat`, `pwsh`, `powershell` (`bash` and `python` via a shebang) |
| `working-directory`, `timeout-minutes`, `continue-on-error` | `dir`, `timeout`, `catchError` |
| job `if:` | `when { beforeAgent true; expression { ... } }` |
| step `if:` | `script { if (...) { ... } }` |
| step `if: always()` / `failure()` / `cancelled()` | stage `post { always / failure / aborted }` |
| `actions/checkout` | `checkout scm` |
| `actions/upload-artifact` / `download-artifact` | `stash` + `archiveArtifacts` / `unstash` |
| `docker://image` | `docker.image(...).inside { sh ... }` |
| `on.schedule` | `triggers { cron('TZ=UTC\n...') }` |
| `on.workflow_dispatch.inputs` | `parameters { string / booleanParam / choice }` |
| `concurrency` | `options { disableConcurrentBuilds() }` |

Expressions are translated into Groovy. For example, `github.ref_name` becomes
`env.BRANCH_NAME`, `github.sha` becomes `env.GIT_COMMIT`, `inputs.x` becomes
`params.x`, `startsWith(a, b)` becomes `a.startsWith(b)`, and
`cond && 'a' || 'b'` becomes `cond ? 'a' : 'b'`. Inside shell scripts,
expressions are turned into shell variables (`${GIT_COMMIT}`) when they can be,
so secrets are never put into the script text by Groovy.

### Not supported (yet)

Reusable workflows, `actions/setup-*` and `actions/cache`, other marketplace
actions, service containers, job/step outputs, matrix `include`, dynamic
matrices, `hashFiles()`, and push/pull_request triggers. Push and pull_request
builds come from using a Multibranch Pipeline. Each of these produces a TODO
and a warning.

## Development

```sh
go test ./...
go test ./internal/convert -update   # regenerate golden Jenkinsfiles in testdata/
make lint        # gofmt, go vet, golangci-lint, go mod tidy, actionlint, Jenkinsfile freshness
make cover       # tests with -race; fails below 80% coverage; writes coverage.html
make dist        # cross-compile release binaries and SHA256SUMS into dist/
make jenkinsfile # regenerate ./Jenkinsfile after editing .github/workflows/ci.yml
```

### CI/CD

[`.github/workflows/ci.yml`](.github/workflows/ci.yml) runs on pushes to
`main` and `dev`, on pull requests and on `v*` tags:

| Job | What it does |
| --- | --- |
| Lint | `make lint` in `golangci/golangci-lint` |
| Test | `make cover` on Go 1.22 (the go.mod minimum) and 1.27, uploading coverage reports |
| Build binaries | `make dist` for linux, darwin and windows |
| Jenkins end-to-end | `make e2e`: lints every generated Jenkinsfile and builds the sample on Jenkins in podman |
| Release | for `v*` tags, publishes the binaries as a GitHub release |

### Versioning and releases

jenkinsfy follows [Semantic Versioning](https://semver.org). Releases are
`vX.Y.Z` git tags (pre-releases look like `v1.0.0-rc.1`), and changes are
recorded in [CHANGELOG.md](CHANGELOG.md) using the
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format.

1. Add each change under `## [Unreleased]` in `CHANGELOG.md`, in the same PR.
2. To release, run `make release V=0.2.0` on an up-to-date `main` branch. This
   moves the Unreleased entries into `## [0.2.0] - <today>`, updates the
   compare links, commits `Release v0.2.0`, and creates an annotated tag. It
   doesn't push anything.
3. Push with `git push origin main v0.2.0`. CI builds the binaries, checks
   that they report `v0.2.0`, and publishes a GitHub release whose notes are
   that changelog section (marked as a pre-release when the version has a
   `-`). Tags without a changelog entry fail the release job.

`jenkinsfy -version` prints the release version. It's stamped into release
binaries and read from module info for `go install ...@vX.Y.Z`, and is `dev`
for local builds.

The repository's [`Jenkinsfile`](Jenkinsfile) is that same workflow converted
by jenkinsfy (`make jenkinsfile`). The lint job fails if it gets out of date.
To run it, use a Multibranch Pipeline and set up:

- agents labelled `linux` with Docker, for the containerized stages;
- `go`, `podman` and `make` on the agent that runs the end-to-end stage;
- a secret-text credential `GITHUB_TOKEN` and tag discovery, for releases.

### Checking against a real Jenkins

`test/jenkins/verify.sh` runs a throwaway Jenkins LTS in podman on
http://127.0.0.1:8080 (user and password `admin`, with dummy secret-text
credentials) and checks the converter against it:

```sh
test/jenkins/verify.sh all    # start Jenkins, lint, then build the sample
test/jenkins/verify.sh lint   # run Jenkins' declarative linter on every generated Jenkinsfile
test/jenkins/verify.sh run [workflow.yml]   # convert and build as a multibranch job (default: test/jenkins/sample.yml)
test/jenkins/verify.sh down   # remove the container (the data volume is kept)
```

`run` maps `ubuntu-latest` to the built-in node, so the workflow must run on a
plain Linux controller without Docker. The build log is saved to
`test/jenkins/last-console.txt`.
