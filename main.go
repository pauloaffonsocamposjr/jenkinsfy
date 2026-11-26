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
	"strings"

	"github.com/pauloaffonsocamposjr/jenkinsfy/internal/convert"
	"github.com/pauloaffonsocamposjr/jenkinsfy/internal/workflow"
)

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
		defer f.Close()
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
	} else {
		io.WriteString(stdout, res.Jenkinsfile)
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
