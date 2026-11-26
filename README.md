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
```

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
