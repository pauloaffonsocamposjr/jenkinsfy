// Package expr translates GitHub Actions expressions (`${{ ... }}`) into
// Groovy code usable in a declarative Jenkinsfile, and shell-friendly
// environment variable references where possible.
package expr

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Translator converts expressions and remembers side information discovered
// along the way: which secrets were referenced and which constructs could not
// be translated faithfully.
type Translator struct {
	secrets  map[string]bool
	warnings []string
	seenWarn map[string]bool
}

// New returns an empty Translator.
func New() *Translator {
	return &Translator{secrets: map[string]bool{}, seenWarn: map[string]bool{}}
}

// Secrets returns the sorted names of every secret referenced so far.
func (t *Translator) Secrets() []string {
	out := make([]string, 0, len(t.secrets))
	for s := range t.secrets {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Warnings returns the de-duplicated warnings collected so far.
func (t *Translator) Warnings() []string { return append([]string(nil), t.warnings...) }

// Warn records a warning once.
func (t *Translator) Warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if t.seenWarn[msg] {
		return
	}
	t.seenWarn[msg] = true
	t.warnings = append(t.warnings, msg)
}

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var wrapped = regexp.MustCompile(`^\s*\$\{\{(.*)\}\}\s*$`)

// Unwrap strips a surrounding `${{ }}` if the whole string is one expression.
// `if:` conditions may be written with or without the wrapper.
func Unwrap(s string) string {
	if m := wrapped.FindStringSubmatch(s); m != nil && !strings.Contains(m[1], "}}") {
		return strings.TrimSpace(m[1])
	}
	return strings.TrimSpace(s)
}

// Condition translates an `if:` condition into a Groovy boolean expression.
func (t *Translator) Condition(src string) (string, error) {
	n, err := parse(Unwrap(src))
	if err != nil {
		return "", err
	}
	return t.groovyIn(n, false)
}

// Groovy translates a bare expression (no `${{ }}`) into Groovy code.
func (t *Translator) Groovy(src string) (string, error) {
	n, err := parse(src)
	if err != nil {
		return "", err
	}
	return t.groovy(n)
}

// Segment is a piece of an interpolated string: either literal text or
// Groovy code that must be evaluated with `${...}`.
type Segment struct {
	Text   string
	Groovy bool
}

var interp = regexp.MustCompile(`\$\{\{(.*?)\}\}`)

// Interpolate splits s into literal and Groovy segments. When shell is true,
// expressions that map onto an environment variable are emitted as literal
// shell references (`${VAR}`) so no Groovy interpolation is needed; this is
// what Jenkins recommends for secrets.
func (t *Translator) Interpolate(s string, shell bool) []Segment {
	var segs []Segment
	add := func(text string, groovy bool) {
		if text == "" {
			return
		}
		if n := len(segs); n > 0 && segs[n-1].Groovy == groovy && !groovy {
			segs[n-1].Text += text
			return
		}
		segs = append(segs, Segment{Text: text, Groovy: groovy})
	}
	last := 0
	for _, loc := range interp.FindAllStringSubmatchIndex(s, -1) {
		add(s[last:loc[0]], false)
		last = loc[1]
		src := strings.TrimSpace(s[loc[2]:loc[3]])
		n, err := parse(src)
		if err != nil {
			t.Warn("could not parse expression %q: %v", src, err)
			add(s[loc[0]:loc[1]], false)
			continue
		}
		if r, ok := n.(*refNode); ok && shell {
			if sh := t.resolve(r.path).shell; sh != "" {
				add(sh, false)
				continue
			}
		}
		code, err := t.groovy(n)
		if err != nil {
			t.Warn("could not translate expression %q: %v", src, err)
			add(s[loc[0]:loc[1]], false)
			continue
		}
		add(code, true)
	}
	add(s[last:], false)
	return segs
}

// HasExpression reports whether s contains a `${{ }}` expression.
func HasExpression(s string) bool { return interp.MatchString(s) }

// ---------------------------------------------------------------------------
// Context resolution

type resolved struct {
	groovy string // Groovy expression
	shell  string // shell form, empty if none
}

func envRef(name string) resolved {
	return resolved{groovy: "env." + name, shell: "${" + name + "}"}
}

// EnvName turns an arbitrary identifier into a valid environment variable
// name (dashes are not allowed in shell variable names).
func EnvName(s string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(s)
}

var githubContext = map[string]string{
	"sha":         "GIT_COMMIT",
	"ref_name":    "BRANCH_NAME",
	"head_ref":    "CHANGE_BRANCH",
	"base_ref":    "CHANGE_TARGET",
	"run_number":  "BUILD_NUMBER",
	"run_id":      "BUILD_ID",
	"workspace":   "WORKSPACE",
	"job":         "STAGE_NAME",
	"actor":       "CHANGE_AUTHOR",
	"server_url":  "JENKINS_URL",
	"run_attempt": "BUILD_NUMBER",
}

func (t *Translator) resolve(path []string) resolved {
	full := strings.Join(path, ".")
	switch {
	case len(path) == 2 && (path[0] == "env" || path[0] == "matrix"):
		return envRef(EnvName(path[1]))
	case len(path) == 2 && path[0] == "secrets":
		t.secrets[path[1]] = true
		if path[1] == "GITHUB_TOKEN" {
			t.Warn("secrets.GITHUB_TOKEN has no Jenkins equivalent; create a credential with id 'GITHUB_TOKEN'")
		}
		return envRef(path[1])
	case len(path) == 2 && path[0] == "vars":
		t.Warn("vars.%s: define %s as a global environment variable in Jenkins", path[1], path[1])
		return envRef(path[1])
	case len(path) == 2 && path[0] == "inputs",
		len(path) == 4 && path[0] == "github" && path[1] == "event" && path[2] == "inputs":
		name := path[len(path)-1]
		if !identifier.MatchString(name) {
			// Not a valid Groovy property or shell variable name.
			return resolved{groovy: "params[" + QuoteString(name) + "]"}
		}
		return resolved{groovy: "params." + name, shell: "${" + name + "}"}
	case full == "github.ref":
		return resolved{groovy: "('refs/heads/' + env.BRANCH_NAME)", shell: "refs/heads/${BRANCH_NAME}"}
	case full == "runner.os":
		return resolved{groovy: "(isUnix() ? 'Linux' : 'Windows')"}
	case full == "runner.temp":
		return envRef("WORKSPACE_TMP")
	case len(path) == 2 && path[0] == "github":
		if v, ok := githubContext[path[1]]; ok {
			return envRef(v)
		}
	}
	name := strings.ToUpper(EnvName(strings.Join(path, "_")))
	t.Warn("no Jenkins equivalent for %q; mapped to env.%s", full, name)
	return envRef(name)
}

// ---------------------------------------------------------------------------
// Groovy generation

var postFuncs = map[string]string{
	"success":   "currentBuild.currentResult == 'SUCCESS'",
	"failure":   "currentBuild.currentResult == 'FAILURE'",
	"cancelled": "currentBuild.currentResult == 'ABORTED'",
	"always":    "true",
}

// groovy translates n where a value is expected (interpolation, function
// arguments, comparison operands).
func (t *Translator) groovy(n node) (string, error) { return t.groovyIn(n, true) }

// groovyIn translates n. In a value context GitHub's && and || return one of
// their operands, while Groovy's always return a boolean, so they become
// `?:` and `? :`. In a boolean context (if: conditions) they are kept as-is.
func (t *Translator) groovyIn(n node, value bool) (string, error) {
	switch n := n.(type) {
	case *litNode:
		return n.groovy, nil
	case *refNode:
		return t.resolve(n.path).groovy, nil
	case *notNode:
		inner, err := t.groovyIn(n.x, false)
		if err != nil {
			return "", err
		}
		if needsParens(n.x) {
			inner = "(" + inner + ")"
		}
		return "!" + inner, nil
	case *parenNode:
		inner, err := t.groovyIn(n.x, value)
		if err != nil {
			return "", err
		}
		return "(" + inner + ")", nil
	case *binNode:
		// GitHub's `cond && a || b` idiom yields a value; Groovy's && and ||
		// always yield booleans, so rewrite it as a ternary.
		logical := n.op == "||" || n.op == "&&"
		if n.op == "||" {
			if l, ok := n.l.(*binNode); ok && l.op == "&&" {
				c, err := t.groovyIn(l.l, false)
				if err != nil {
					return "", err
				}
				a, err := t.groovyIn(l.r, value)
				if err != nil {
					return "", err
				}
				b, err := t.groovyIn(n.r, value)
				if err != nil {
					return "", err
				}
				return "(" + c + " ? " + a + " : " + b + ")", nil
			}
		}
		// Operands of && and || inherit the context; comparison operands
		// are always values.
		sub := true
		if logical {
			sub = value
		}
		l, err := t.groovyIn(n.l, sub)
		if err != nil {
			return "", err
		}
		r, err := t.groovyIn(n.r, sub)
		if err != nil {
			return "", err
		}
		switch {
		case value && n.op == "||":
			return "(" + l + " ?: " + r + ")", nil
		case value && n.op == "&&":
			return "(" + l + " ? " + r + " : " + l + ")", nil
		}
		return l + " " + n.op + " " + r, nil
	case *callNode:
		return t.call(n)
	}
	return "", fmt.Errorf("unsupported expression")
}

// needsParens reports whether n renders as a binary Groovy expression that
// must be parenthesised when used as an operand of ! or a method call.
func needsParens(n node) bool {
	switch n := n.(type) {
	case *binNode:
		return true
	case *callNode:
		switch strings.ToLower(n.name) {
		case "success", "failure", "cancelled":
			return true
		}
	}
	return false
}

func (t *Translator) call(n *callNode) (string, error) {
	name := strings.ToLower(n.name)
	args := make([]string, len(n.args))
	for i, a := range n.args {
		g, err := t.groovy(a)
		if err != nil {
			return "", err
		}
		if needsParens(a) {
			g = "(" + g + ")"
		}
		args[i] = g
	}
	want := func(k int) error {
		if len(args) != k {
			return fmt.Errorf("%s() expects %d argument(s), got %d", n.name, k, len(args))
		}
		return nil
	}
	switch name {
	case "success", "failure", "cancelled", "always":
		if err := want(0); err != nil {
			return "", err
		}
		return postFuncs[name], nil
	case "contains", "startswith", "endswith":
		if err := want(2); err != nil {
			return "", err
		}
		method := map[string]string{"contains": "contains", "startswith": "startsWith", "endswith": "endsWith"}[name]
		return args[0] + "." + method + "(" + args[1] + ")", nil
	case "join":
		if len(args) == 1 {
			return args[0] + ".join(',')", nil
		}
		if err := want(2); err != nil {
			return "", err
		}
		return args[0] + ".join(" + args[1] + ")", nil
	case "tojson":
		if err := want(1); err != nil {
			return "", err
		}
		return "groovy.json.JsonOutput.toJson(" + args[0] + ")", nil
	case "fromjson":
		if err := want(1); err != nil {
			return "", err
		}
		t.Warn("fromJSON() is translated to readJSON, which needs the Pipeline Utility Steps plugin")
		return "readJSON(text: " + args[0] + ")", nil
	case "format":
		if len(args) == 0 {
			return "", fmt.Errorf("format() needs at least one argument")
		}
		lit, ok := n.args[0].(*litNode)
		if !ok || lit.str == nil {
			return "", fmt.Errorf("format() needs a literal format string")
		}
		return formatGString(*lit.str, args[1:]), nil
	}
	return "", fmt.Errorf("function %s() is not supported", n.name)
}

var placeholder = regexp.MustCompile(`\{(\d+)\}`)

func formatGString(format string, args []string) string {
	var b strings.Builder
	b.WriteByte('"')
	last := 0
	for _, loc := range placeholder.FindAllStringSubmatchIndex(format, -1) {
		b.WriteString(EscapeGString(format[last:loc[0]]))
		idx := 0
		fmt.Sscanf(format[loc[2]:loc[3]], "%d", &idx)
		if idx < len(args) {
			b.WriteString("${" + args[idx] + "}")
		}
		last = loc[1]
	}
	b.WriteString(EscapeGString(format[last:]))
	b.WriteByte('"')
	return b.String()
}

// QuoteString returns s as a single-quoted Groovy string literal.
func QuoteString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return "'" + r.Replace(s) + "'"
}

// EscapeGString escapes s for use inside a double-quoted Groovy GString.
func EscapeGString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return r.Replace(s)
}

// GroovyString renders segments as a Groovy string literal: single-quoted if
// there is nothing to interpolate, a double-quoted GString otherwise.
func GroovyString(segs []Segment) string {
	if !anyGroovy(segs) {
		var b strings.Builder
		for _, s := range segs {
			b.WriteString(s.Text)
		}
		return QuoteString(b.String())
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, s := range segs {
		if s.Groovy {
			b.WriteString("${" + s.Text + "}")
		} else {
			b.WriteString(EscapeGString(s.Text))
		}
	}
	b.WriteByte('"')
	return b.String()
}

// TripleQuoted renders segments as a multi-line Groovy string: `”'` when
// nothing needs interpolation, `"""` otherwise.
func TripleQuoted(segs []Segment) string {
	if !anyGroovy(segs) {
		var b strings.Builder
		for _, s := range segs {
			b.WriteString(s.Text)
		}
		body := strings.NewReplacer(`\`, `\\`, `'''`, `\'\'\'`).Replace(b.String())
		return "'''" + body + "'''"
	}
	var b strings.Builder
	esc := strings.NewReplacer(`\`, `\\`, `$`, `\$`, `"""`, `\"\"\"`)
	for _, s := range segs {
		if s.Groovy {
			b.WriteString("${" + s.Text + "}")
		} else {
			b.WriteString(esc.Replace(s.Text))
		}
	}
	return `"""` + b.String() + `"""`
}

func anyGroovy(segs []Segment) bool {
	for _, s := range segs {
		if s.Groovy {
			return true
		}
	}
	return false
}
