package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sample = `
name: Demo
on: push
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: make
`

func runCLI(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLIReadsStdin(t *testing.T) {
	code, out, errOut := runCLI(t, sample)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if !strings.Contains(out, "pipeline {") || !strings.Contains(out, "sh 'make'") {
		t.Errorf("unexpected output:\n%s", out)
	}
}

func TestCLIReadsFileAndWritesOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "ci.yml")
	outPath := filepath.Join(dir, "Jenkinsfile")
	if err := os.WriteFile(in, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := runCLI(t, "", "-o", outPath, "-label", "ubuntu-latest=linux", in)
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, errOut)
	}
	if out != "" {
		t.Errorf("stdout should be empty with -o, got %q", out)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "agent { label 'linux' }") {
		t.Errorf("label map not applied:\n%s", data)
	}
}

func TestCLIWarningsAndStrict(t *testing.T) {
	src := strings.Replace(sample, "- run: make", "- uses: acme/thing@v1", 1)

	code, _, errOut := runCLI(t, src)
	if code != 0 || !strings.Contains(errOut, "warning: ") {
		t.Errorf("exit %d, stderr %q; want 0 with a warning", code, errOut)
	}
	code, _, errOut = runCLI(t, src, "-q")
	if code != 0 || errOut != "" {
		t.Errorf("-q: exit %d, stderr %q; want 0 and silence", code, errOut)
	}
	code, _, _ = runCLI(t, src, "-strict")
	if code != 2 {
		t.Errorf("-strict: exit %d, want 2", code)
	}
	code, _, _ = runCLI(t, sample, "-strict")
	if code != 0 {
		t.Errorf("-strict without warnings: exit %d, want 0", code)
	}
}

func TestCLIErrors(t *testing.T) {
	tests := []struct {
		name  string
		stdin string
		args  []string
		want  string
	}{
		{"missing file", "", []string{"does-not-exist.yml"}, "no such file"},
		{"invalid workflow", "on: push\n", nil, "no jobs"},
		{"bad label", sample, []string{"-label", "nope"}, "FROM=TO"},
		{"too many args", "", []string{"a.yml", "b.yml"}, "Usage"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, errOut := runCLI(t, tt.stdin, tt.args...)
			if code != 1 || !strings.Contains(errOut, tt.want) {
				t.Errorf("exit %d, stderr %q; want 1 containing %q", code, errOut, tt.want)
			}
		})
	}
}
