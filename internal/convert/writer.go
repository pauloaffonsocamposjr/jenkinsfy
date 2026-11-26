package convert

import (
	"fmt"
	"strings"
)

const indentUnit = "    "

// writer builds indented Groovy source.
type writer struct {
	b     strings.Builder
	depth int
}

func (w *writer) pad() string { return strings.Repeat(indentUnit, w.depth) }

// line writes one indented line. The text may itself contain newlines (for
// multi-line string literals); those are written verbatim.
func (w *writer) line(format string, args ...any) {
	w.b.WriteString(w.pad())
	if len(args) == 0 {
		w.b.WriteString(format)
	} else {
		fmt.Fprintf(&w.b, format, args...)
	}
	w.b.WriteByte('\n')
}

// open writes `<text> {` and increases the indentation.
func (w *writer) open(format string, args ...any) {
	w.line(format+" {", args...)
	w.depth++
}

// close decreases the indentation and writes `}`.
func (w *writer) close() {
	w.depth--
	w.line("}")
}

// todo writes a TODO comment that users can grep for.
func (w *writer) todo(format string, args ...any) {
	w.line("// TODO(jenkinsfy): "+format, args...)
}

func (w *writer) String() string { return w.b.String() }
