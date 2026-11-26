// Command jenkinsfy converts a GitHub Actions workflow into a declarative
// Jenkinsfile.
//
// Usage:
//
//	jenkinsfy [flags] [workflow.yml]
//
// The workflow is read from the given file, or from stdin when the file is
// omitted or "-". The Jenkinsfile is written to stdout unless -o is given.
// Conversion warnings are printed to stderr.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/pauloaffonsocamposjr/jenkinsfy/internal/convert"
	"github.com/pauloaffonsocamposjr/jenkinsfy/internal/workflow"
)

// version is set at release build time with -ldflags "-X main.version=...".
// When empty, the module version recorded by `go install module@version` is
// used, and "dev" for local builds.
var version = ""

// resolveVersion returns the version to report for this binary.
func resolveVersion(ldflags string, info *debug.BuildInfo, ok bool) string {
	if ldflags != "" {
		return ldflags
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

type labelFlag map[string]string

func (l labelFlag) String() string { return "" }

func (l labelFlag) Set(v string) error {
	from, to, ok := strings.Cut(v, "=")
	if !ok || from == "" || to == "" {
		return errors.New("expected FROM=TO, e.g. ubuntu-latest=linux")
	}
	l[from] = to
	return nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("jenkinsfy", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "", "write the Jenkinsfile to `file` instead of stdout")
	strict := fs.Bool("strict", false, "exit with status 2 if the conversion produced warnings")
	quiet := fs.Bool("q", false, "do not print warnings")
	showVersion := fs.Bool("version", false, "print the version and exit")
	labels := labelFlag{}
	fs.Var(labels, "label", "map a GitHub runner label to a Jenkins agent label, as `FROM=TO` (repeatable)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: jenkinsfy [flags] [workflow.yml]")
		fmt.Fprintln(stderr, "Converts a GitHub Actions workflow into a declarative Jenkinsfile.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if *showVersion {
		info, ok := debug.ReadBuildInfo()
		fmt.Fprintln(stdout, "jenkinsfy", resolveVersion(version, info, ok))
		return 0
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return 1
	}

	in := stdin
	if path := fs.Arg(0); path != "" && path != "-" {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(stderr, "jenkinsfy:", err)
			return 1
		}
		defer f.Close() //nolint:errcheck // read-only file
		in = f
	}

	wf, err := workflow.Parse(in)
	if err != nil {
		fmt.Fprintln(stderr, "jenkinsfy:", err)
		return 1
	}
	res, err := convert.Convert(wf, convert.Options{LabelMap: labels})
	if err != nil {
		fmt.Fprintln(stderr, "jenkinsfy:", err)
		return 1
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(res.Jenkinsfile), 0o644); err != nil {
			fmt.Fprintln(stderr, "jenkinsfy:", err)
			return 1
		}
	} else if _, err := io.WriteString(stdout, res.Jenkinsfile); err != nil {
		fmt.Fprintln(stderr, "jenkinsfy:", err)
		return 1
	}
	if !*quiet {
		for _, w := range res.Warnings {
			fmt.Fprintln(stderr, "warning:", w)
		}
	}
	if *strict && len(res.Warnings) > 0 {
		return 2
	}
	return 0
}
