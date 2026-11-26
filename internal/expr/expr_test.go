package expr

import (
	"reflect"
	"strings"
	"testing"
)

func TestCondition(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"github.ref == 'refs/heads/main'", "('refs/heads/' + env.BRANCH_NAME) == 'refs/heads/main'"},
		{"${{ github.ref_name == 'main' }}", "env.BRANCH_NAME == 'main'"},
		{"success()", "currentBuild.currentResult == 'SUCCESS'"},
		{"failure()", "currentBuild.currentResult == 'FAILURE'"},
		{"always()", "true"},
		{"cancelled()", "currentBuild.currentResult == 'ABORTED'"},
		{"!cancelled()", "!(currentBuild.currentResult == 'ABORTED')"},
		{"env.DEPLOY == 'true' && matrix.os != 'windows'", "env.DEPLOY == 'true' && env.os != 'windows'"},
		{"!(a.b || c.d)", "!(env.A_B || env.C_D)"},
		{"startsWith(github.ref_name, 'release/')", "env.BRANCH_NAME.startsWith('release/')"},
		{"endsWith(env.FILE, '.zip')", "env.FILE.endsWith('.zip')"},
		{"contains(github.ref_name, 'feat')", "env.BRANCH_NAME.contains('feat')"},
		{"inputs.deploy == true", "params.deploy == true"},
		{"github.event.inputs.target == 'prod'", "params.target == 'prod'"},
		{"inputs.dry-run", "params['dry-run']"},
		{"matrix.node-version >= 20", "env.node_version >= 20"},
		{"runner.os == 'Linux'", "(isUnix() ? 'Linux' : 'Windows') == 'Linux'"},
		{"env.X == 'it''s'", `env.X == 'it\'s'`},
		{"github.sha", "env.GIT_COMMIT"},
		{"github.run_number > 10", "env.BUILD_NUMBER > 10"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := New().Condition(tt.in)
			if err != nil {
				t.Fatalf("Condition(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Condition(%q)\n got: %s\nwant: %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestNotWrapsBinaryOperand(t *testing.T) {
	got, err := New().Condition("!(github.ref_name == 'main')")
	if err != nil {
		t.Fatal(err)
	}
	if want := "!(env.BRANCH_NAME == 'main')"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTernaryIdiom(t *testing.T) {
	got, err := New().Groovy("github.ref_name == 'main' && 'prod' || 'dev'")
	if err != nil {
		t.Fatal(err)
	}
	if want := "(env.BRANCH_NAME == 'main' ? 'prod' : 'dev')"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestLogicalOperatorsInValueContext(t *testing.T) {
	tests := []struct{ in, want string }{
		{"inputs.greeting || 'hello'", "(params.greeting ?: 'hello')"},
		{"env.A && env.B", "(env.A ? env.B : env.A)"},
		{"inputs.a || inputs.b || 'c'", "((params.a ?: params.b) ?: 'c')"},
		// Comparisons are still booleans.
		{"env.A == 'x' || env.B", "(env.A == 'x' ?: env.B)"},
	}
	for _, tt := range tests {
		got, err := New().Groovy(tt.in)
		if err != nil {
			t.Fatalf("Groovy(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("Groovy(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
	// In conditions the boolean operators are kept.
	got, err := New().Condition("inputs.a || env.B && !env.C")
	if err != nil {
		t.Fatal(err)
	}
	if want := "params.a || env.B && !env.C"; got != want {
		t.Errorf("Condition = %s, want %s", got, want)
	}
}

func TestInterpolateDefaultValue(t *testing.T) {
	got := TripleQuoted(New().Interpolate("echo ${{ inputs.greeting || 'hello' }} $APP", true))
	if want := `"""echo ${(params.greeting ?: 'hello')} \$APP"""`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestFormatAndJSON(t *testing.T) {
	tr := New()
	got, err := tr.Groovy("format('v{0}-{1}', github.run_number, github.sha)")
	if err != nil {
		t.Fatal(err)
	}
	if want := `"v${env.BUILD_NUMBER}-${env.GIT_COMMIT}"`; got != want {
		t.Errorf("format: got %s, want %s", got, want)
	}
	got, err = tr.Groovy("toJSON(matrix.os)")
	if err != nil {
		t.Fatal(err)
	}
	if want := "groovy.json.JsonOutput.toJson(env.os)"; got != want {
		t.Errorf("toJSON: got %s, want %s", got, want)
	}
}

func TestConditionErrors(t *testing.T) {
	for _, in := range []string{
		"hashFiles('**/go.sum') != ''", // unsupported function
		"github.ref ==",                // dangling operator
		"'unterminated",                // bad string
		"contains(a)",                  // wrong arity
		"a # b",                        // bad character
		"",                             // empty
	} {
		if _, err := New().Condition(in); err == nil {
			t.Errorf("Condition(%q) succeeded, want error", in)
		}
	}
}

func TestSecretsAreCollected(t *testing.T) {
	tr := New()
	if _, err := tr.Condition("secrets.B != '' && secrets.A != ''"); err != nil {
		t.Fatal(err)
	}
	tr.Interpolate("token=${{ secrets.C }}", true)
	if got, want := tr.Secrets(), []string{"A", "B", "C"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Secrets() = %v, want %v", got, want)
	}
}

func TestUnknownContextWarns(t *testing.T) {
	tr := New()
	got, err := tr.Condition("github.event_name == 'push'")
	if err != nil {
		t.Fatal(err)
	}
	if got != "env.GITHUB_EVENT_NAME == 'push'" {
		t.Errorf("got %s", got)
	}
	tr.Condition("github.event_name == 'pull_request'") // same warning again
	if w := tr.Warnings(); len(w) != 1 || !strings.Contains(w[0], "github.event_name") {
		t.Errorf("warnings = %v, want one about github.event_name", w)
	}
}

func TestInterpolateShell(t *testing.T) {
	tr := New()
	segs := tr.Interpolate("echo ${{ github.sha }} on ${{ github.ref }} for ${{ inputs.target }}", true)
	want := []Segment{{Text: "echo ${GIT_COMMIT} on refs/heads/${BRANCH_NAME} for ${target}"}}
	if !reflect.DeepEqual(segs, want) {
		t.Errorf("segments = %+v, want %+v", segs, want)
	}
	if got := TripleQuoted(segs); got != "'''echo ${GIT_COMMIT} on refs/heads/${BRANCH_NAME} for ${target}'''" {
		t.Errorf("TripleQuoted = %s", got)
	}
}

func TestInterpolateFallsBackToGroovy(t *testing.T) {
	tr := New()
	segs := tr.Interpolate("echo $HOME ${{ runner.os }}", true)
	want := []Segment{{Text: "echo $HOME "}, {Text: "(isUnix() ? 'Linux' : 'Windows')", Groovy: true}}
	if !reflect.DeepEqual(segs, want) {
		t.Fatalf("segments = %+v, want %+v", segs, want)
	}
	// Shell variables must be escaped once the string becomes a GString.
	if got, want := TripleQuoted(segs), `"""echo \$HOME ${(isUnix() ? 'Linux' : 'Windows')}"""`; got != want {
		t.Errorf("TripleQuoted = %s, want %s", got, want)
	}
	if got, want := GroovyString(segs), `"echo \$HOME ${(isUnix() ? 'Linux' : 'Windows')}"`; got != want {
		t.Errorf("GroovyString = %s, want %s", got, want)
	}
}

func TestInterpolateNonShell(t *testing.T) {
	tr := New()
	got := GroovyString(tr.Interpolate("v1.${{ github.run_number }}", false))
	if want := `"v1.${env.BUILD_NUMBER}"`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if got := GroovyString(tr.Interpolate("plain", false)); got != "'plain'" {
		t.Errorf("plain string = %s", got)
	}
}

func TestInterpolateKeepsBrokenExpression(t *testing.T) {
	tr := New()
	segs := tr.Interpolate("x ${{ hashFiles('a') }} y", true)
	if got := TripleQuoted(segs); got != "'''x ${{ hashFiles('a') }} y'''" {
		t.Errorf("got %s", got)
	}
	if len(tr.Warnings()) != 1 {
		t.Errorf("warnings = %v, want 1", tr.Warnings())
	}
}

func TestQuoting(t *testing.T) {
	if got, want := QuoteString(`it's a \ test`+"\n"), `'it\'s a \\ test\n'`; got != want {
		t.Errorf("QuoteString = %s, want %s", got, want)
	}
	if got, want := EscapeGString(`"$x\`), `\"\$x\\`; got != want {
		t.Errorf("EscapeGString = %s, want %s", got, want)
	}
	if got, want := TripleQuoted([]Segment{{Text: `a ''' b \n`}}), `'''a \'\'\' b \\n'''`; got != want {
		t.Errorf("TripleQuoted = %s, want %s", got, want)
	}
}

func TestUnwrap(t *testing.T) {
	tests := map[string]string{
		"${{ always() }}":      "always()",
		"  always()  ":         "always()",
		"${{ a }} && ${{ b }}": "${{ a }} && ${{ b }}",
		"${{github.ref_name}}": "github.ref_name",
	}
	for in, want := range tests {
		if got := Unwrap(in); got != want {
			t.Errorf("Unwrap(%q) = %q, want %q", in, got, want)
		}
	}
}
