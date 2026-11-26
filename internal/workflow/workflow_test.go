package workflow

import (
	"reflect"
	"strings"
	"testing"
)

func mustParse(t *testing.T, src string) *Workflow {
	t.Helper()
	wf, err := ParseBytes([]byte(src))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	return wf
}

const minimalJobs = `
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - run: echo hi
`

func TestTriggerShapes(t *testing.T) {
	tests := []struct {
		name string
		on   string
		want []string
	}{
		{"scalar", "on: push", []string{"push"}},
		{"list", "on: [push, pull_request]", []string{"push", "pull_request"}},
		{"mapping", "on:\n  push:\n  workflow_dispatch:", []string{"push", "workflow_dispatch"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := mustParse(t, tt.on+minimalJobs)
			if !reflect.DeepEqual(wf.On.Events, tt.want) {
				t.Errorf("events = %v, want %v", wf.On.Events, tt.want)
			}
		})
	}
}

func TestTriggerDetails(t *testing.T) {
	wf := mustParse(t, `
on:
  push:
    branches: [main, 'release/**']
  schedule:
    - cron: '0 0 * * *'
    - cron: '30 12 * * 5'
  workflow_dispatch:
    inputs:
      env:
        description: Where to deploy
        type: choice
        options: [dev, prod]
        default: dev
      force:
        type: boolean
`+minimalJobs)

	if got := wf.On.Branches["push"]; !reflect.DeepEqual(got, []string{"main", "release/**"}) {
		t.Errorf("push branches = %v", got)
	}
	if got := wf.On.Schedules; !reflect.DeepEqual(got, []string{"0 0 * * *", "30 12 * * 5"}) {
		t.Errorf("schedules = %v", got)
	}
	want := []Input{
		{Name: "env", Description: "Where to deploy", Type: "choice", Options: []string{"dev", "prod"}, Default: "dev"},
		{Name: "force", Type: "boolean"},
	}
	if !reflect.DeepEqual(wf.On.Inputs, want) {
		t.Errorf("inputs = %+v, want %+v", wf.On.Inputs, want)
	}
	if !wf.On.Has("schedule") || wf.On.Has("pull_request") {
		t.Errorf("Has() misreports events: %v", wf.On.Events)
	}
}

func TestJobsKeepSourceOrder(t *testing.T) {
	wf := mustParse(t, `
on: push
jobs:
  zeta:
    runs-on: ubuntu-latest
    steps: [{run: a}]
  alpha:
    runs-on: ubuntu-latest
    needs: zeta
    steps: [{run: b}]
  mid:
    runs-on: ubuntu-latest
    needs: [zeta, alpha]
    steps: [{run: c}]
`)
	var ids []string
	for _, j := range wf.Jobs {
		ids = append(ids, j.ID)
	}
	if want := []string{"zeta", "alpha", "mid"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("job order = %v, want %v", ids, want)
	}
	if got := wf.Jobs[1].Needs; !reflect.DeepEqual([]string(got), []string{"zeta"}) {
		t.Errorf("scalar needs = %v", got)
	}
	if got := wf.Jobs[2].Needs; !reflect.DeepEqual([]string(got), []string{"zeta", "alpha"}) {
		t.Errorf("list needs = %v", got)
	}
}

func TestRunsOnShapes(t *testing.T) {
	tests := []struct {
		runsOn     string
		wantLabels []string
		wantGroup  string
	}{
		{"ubuntu-latest", []string{"ubuntu-latest"}, ""},
		{"[self-hosted, linux, x64]", []string{"self-hosted", "linux", "x64"}, ""},
		{"{group: big, labels: [gpu]}", []string{"gpu"}, "big"},
	}
	for _, tt := range tests {
		t.Run(tt.runsOn, func(t *testing.T) {
			wf := mustParse(t, "on: push\njobs:\n  j:\n    runs-on: "+tt.runsOn+"\n    steps: [{run: x}]\n")
			ro := wf.Jobs[0].RunsOn
			if !reflect.DeepEqual(ro.Labels, tt.wantLabels) || ro.Group != tt.wantGroup {
				t.Errorf("runs-on = %+v, want labels %v group %q", ro, tt.wantLabels, tt.wantGroup)
			}
		})
	}
}

func TestEnvIsOrderedAndStringified(t *testing.T) {
	wf := mustParse(t, `
on: push
env:
  Z: last-alphabetically
  A: 1
  B: true
`+strings.TrimPrefix(minimalJobs, "\n"))
	want := OrderedMap{{"Z", "last-alphabetically"}, {"A", "1"}, {"B", "true"}}
	if !reflect.DeepEqual(wf.Env, want) {
		t.Errorf("env = %v, want %v", wf.Env, want)
	}
	if v, ok := wf.Env.Get("A"); !ok || v != "1" {
		t.Errorf("Get(A) = %q, %v", v, ok)
	}
	if _, ok := wf.Env.Get("missing"); ok {
		t.Error("Get(missing) reported present")
	}
}

func TestMatrix(t *testing.T) {
	wf := mustParse(t, `
on: push
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest]
        go: [1.21, 1.22]
        exclude:
          - os: macos-latest
            go: 1.21
        include:
          - os: windows-latest
            go: 1.22
    steps: [{run: go test}]
`)
	s := wf.Jobs[0].Strategy
	if s == nil || s.Matrix == nil {
		t.Fatal("strategy.matrix not parsed")
	}
	wantAxes := []Axis{{"os", []string{"ubuntu-latest", "macos-latest"}}, {"go", []string{"1.21", "1.22"}}}
	if !reflect.DeepEqual(s.Matrix.Axes, wantAxes) {
		t.Errorf("axes = %+v, want %+v", s.Matrix.Axes, wantAxes)
	}
	if len(s.Matrix.Exclude) != 1 || len(s.Matrix.Include) != 1 {
		t.Errorf("exclude/include = %v / %v", s.Matrix.Exclude, s.Matrix.Include)
	}
	if s.FailFast == nil || *s.FailFast {
		t.Errorf("fail-fast = %v, want false", s.FailFast)
	}
}

func TestDynamicMatrix(t *testing.T) {
	wf := mustParse(t, `
on: push
jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix: ${{ fromJSON(needs.setup.outputs.matrix) }}
    steps: [{run: x}]
`)
	if got := wf.Jobs[0].Strategy.Matrix.Expression; got != "${{ fromJSON(needs.setup.outputs.matrix) }}" {
		t.Errorf("matrix expression = %q", got)
	}
}

func TestContainerShapes(t *testing.T) {
	wf := mustParse(t, `
on: push
jobs:
  a:
    runs-on: ubuntu-latest
    container: node:20
    steps: [{run: x}]
  b:
    runs-on: ubuntu-latest
    container:
      image: golang:1.22
      options: --user root
      env:
        CGO_ENABLED: "0"
    services:
      redis:
        image: redis
    steps: [{run: x}]
`)
	if got := wf.Jobs[0].Container; got == nil || got.Image != "node:20" {
		t.Errorf("scalar container = %+v", got)
	}
	b := wf.Jobs[1]
	if b.Container == nil || b.Container.Image != "golang:1.22" || b.Container.Options != "--user root" {
		t.Errorf("mapping container = %+v", b.Container)
	}
	if v, _ := b.Container.Env.Get("CGO_ENABLED"); v != "0" {
		t.Errorf("container env CGO_ENABLED = %q", v)
	}
	if b.Services.IsZero() {
		t.Error("services not recorded")
	}
	if !wf.Jobs[0].Services.IsZero() {
		t.Error("job without services reports services")
	}
}

func TestConcurrency(t *testing.T) {
	wf := mustParse(t, "on: push\nconcurrency:\n  group: ci\n  cancel-in-progress: true\n"+strings.TrimPrefix(minimalJobs, "\n"))
	if wf.Concurrency == nil || wf.Concurrency.Group != "ci" || !wf.Concurrency.CancelInProgress {
		t.Errorf("concurrency = %+v", wf.Concurrency)
	}
	wf = mustParse(t, "on: push\nconcurrency: deploy\n"+strings.TrimPrefix(minimalJobs, "\n"))
	if wf.Concurrency == nil || wf.Concurrency.Group != "deploy" || wf.Concurrency.CancelInProgress {
		t.Errorf("scalar concurrency = %+v", wf.Concurrency)
	}
}

func TestStepFields(t *testing.T) {
	wf := mustParse(t, `
on: push
jobs:
  j:
    runs-on: ubuntu-latest
    steps:
      - id: build
        name: Build it
        if: success()
        run: make
        shell: bash
        working-directory: src
        continue-on-error: true
        timeout-minutes: 5
        env: {A: b}
      - uses: actions/upload-artifact@v4
        with:
          name: out
          path: dist/
`)
	got := wf.Jobs[0].Steps
	want := []Step{
		{ID: "build", Name: "Build it", If: "success()", Run: "make", Shell: "bash", WorkingDirectory: "src",
			ContinueOnError: true, TimeoutMinutes: 5, Env: OrderedMap{{"A", "b"}}},
		{Uses: "actions/upload-artifact@v4", With: OrderedMap{{"name", "out"}, {"path", "dist/"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("steps =\n%+v\nwant\n%+v", got, want)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"invalid yaml", "on: [push\n", "parse workflow"},
		{"no jobs", "on: push\n", "no jobs"},
		{"unknown needs", "on: push\njobs:\n  a:\n    runs-on: x\n    needs: ghost\n    steps: [{run: x}]\n", `unknown job "ghost"`},
		{"no steps", "on: push\njobs:\n  a:\n    runs-on: x\n", "has no steps"},
		{"bad matrix axis", "on: push\njobs:\n  a:\n    runs-on: x\n    strategy:\n      matrix:\n        os: linux\n    steps: [{run: x}]\n", "must be a list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseBytes([]byte(tt.src))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestReusableWorkflowJobNeedsNoSteps(t *testing.T) {
	wf := mustParse(t, "on: push\njobs:\n  call:\n    uses: org/repo/.github/workflows/ci.yml@main\n")
	if wf.Jobs[0].Uses == "" {
		t.Error("job.uses not parsed")
	}
}
