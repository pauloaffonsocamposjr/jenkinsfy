// Package workflow parses GitHub Actions workflow files into a typed model.
//
// GitHub workflows allow many fields to take several shapes (for example
// `on: push`, `on: [push, pull_request]` or a full mapping). The types in this
// package normalise those shapes so the converter only deals with one form.
package workflow

import (
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Workflow is a parsed GitHub Actions workflow.
type Workflow struct {
	Name        string       `yaml:"name"`
	On          Triggers     `yaml:"on"`
	Env         OrderedMap   `yaml:"env"`
	Defaults    Defaults     `yaml:"defaults"`
	Concurrency *Concurrency `yaml:"concurrency"`
	Permissions yaml.Node    `yaml:"permissions"`
	Jobs        Jobs         `yaml:"jobs"`
}

// Concurrency mirrors `concurrency`, which may be a group name or a mapping.
type Concurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress bool   `yaml:"cancel-in-progress"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *Concurrency) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		c.Group = n.Value
		return nil
	}
	type plain Concurrency
	return n.Decode((*plain)(c))
}

// Defaults mirrors the `defaults` key at workflow or job level.
type Defaults struct {
	Run struct {
		Shell            string `yaml:"shell"`
		WorkingDirectory string `yaml:"working-directory"`
	} `yaml:"run"`
}

// Job is a single entry under `jobs`.
type Job struct {
	ID              string     `yaml:"-"`
	Name            string     `yaml:"name"`
	RunsOn          RunsOn     `yaml:"runs-on"`
	Needs           StringList `yaml:"needs"`
	If              string     `yaml:"if"`
	Env             OrderedMap `yaml:"env"`
	Defaults        Defaults   `yaml:"defaults"`
	Container       *Container `yaml:"container"`
	Services        yaml.Node  `yaml:"services"`
	Strategy        *Strategy  `yaml:"strategy"`
	Steps           []Step     `yaml:"steps"`
	TimeoutMinutes  int        `yaml:"timeout-minutes"`
	ContinueOnError bool       `yaml:"continue-on-error"`
	Outputs         OrderedMap `yaml:"outputs"`
	Environment     yaml.Node  `yaml:"environment"`
	Uses            string     `yaml:"uses"` // reusable workflow call
}

// Step is a single entry in a job's `steps`.
type Step struct {
	ID               string     `yaml:"id"`
	Name             string     `yaml:"name"`
	If               string     `yaml:"if"`
	Uses             string     `yaml:"uses"`
	Run              string     `yaml:"run"`
	Shell            string     `yaml:"shell"`
	With             OrderedMap `yaml:"with"`
	Env              OrderedMap `yaml:"env"`
	WorkingDirectory string     `yaml:"working-directory"`
	ContinueOnError  bool       `yaml:"continue-on-error"`
	TimeoutMinutes   int        `yaml:"timeout-minutes"`
}

// Strategy mirrors `jobs.<id>.strategy`.
type Strategy struct {
	Matrix      *Matrix `yaml:"matrix"`
	FailFast    *bool   `yaml:"fail-fast"`
	MaxParallel int     `yaml:"max-parallel"`
}

// Axis is one dimension of a build matrix.
type Axis struct {
	Name   string
	Values []string
}

// Matrix is a build matrix: a list of axes plus include/exclude combinations.
type Matrix struct {
	Axes    []Axis
	Include []OrderedMap
	Exclude []OrderedMap
	// Expression is set when the whole matrix is an expression, e.g.
	// `matrix: ${{ fromJSON(needs.setup.outputs.matrix) }}`.
	Expression string
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *Matrix) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		m.Expression = n.Value
		return nil
	}
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: matrix must be a mapping", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		switch key {
		case "include", "exclude":
			var list []OrderedMap
			if err := val.Decode(&list); err != nil {
				return fmt.Errorf("line %d: matrix.%s: %w", val.Line, key, err)
			}
			if key == "include" {
				m.Include = list
			} else {
				m.Exclude = list
			}
		default:
			var values []string
			if val.Kind != yaml.SequenceNode {
				return fmt.Errorf("line %d: matrix axis %q must be a list", val.Line, key)
			}
			for _, item := range val.Content {
				if item.Kind != yaml.ScalarNode {
					return fmt.Errorf("line %d: matrix axis %q: only scalar values are supported", item.Line, key)
				}
				values = append(values, item.Value)
			}
			m.Axes = append(m.Axes, Axis{Name: key, Values: values})
		}
	}
	return nil
}

// Container mirrors `jobs.<id>.container`, which may be a string or mapping.
type Container struct {
	Image   string     `yaml:"image"`
	Options string     `yaml:"options"`
	Env     OrderedMap `yaml:"env"`
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *Container) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		c.Image = n.Value
		return nil
	}
	type plain Container
	return n.Decode((*plain)(c))
}

// RunsOn holds the labels from `runs-on`, which may be a string, a list or a
// mapping with `group`/`labels`.
type RunsOn struct {
	Labels []string
	Group  string
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (r *RunsOn) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode, yaml.SequenceNode:
		var l StringList
		if err := l.UnmarshalYAML(n); err != nil {
			return err
		}
		r.Labels = l
		return nil
	case yaml.MappingNode:
		var v struct {
			Group  string     `yaml:"group"`
			Labels StringList `yaml:"labels"`
		}
		if err := n.Decode(&v); err != nil {
			return err
		}
		r.Group, r.Labels = v.Group, v.Labels
		return nil
	}
	return fmt.Errorf("line %d: unsupported runs-on value", n.Line)
}

// StringList accepts either a single string or a list of strings.
type StringList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringList) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*s = StringList{n.Value}
		return nil
	case yaml.SequenceNode:
		var out []string
		if err := n.Decode(&out); err != nil {
			return err
		}
		*s = out
		return nil
	}
	return fmt.Errorf("line %d: expected a string or a list of strings", n.Line)
}

// KV is one entry in an OrderedMap.
type KV struct {
	Key   string
	Value string
}

// OrderedMap is a string-to-string mapping that remembers key order, so the
// generated Jenkinsfile is deterministic and follows the source layout.
type OrderedMap []KV

// UnmarshalYAML implements yaml.Unmarshaler.
func (m *OrderedMap) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: expected a mapping", n.Line)
	}
	out := make(OrderedMap, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if v.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: value of %q must be a scalar", v.Line, k.Value)
		}
		out = append(out, KV{Key: k.Value, Value: v.Value})
	}
	*m = out
	return nil
}

// Get returns the value for key and whether it was present.
func (m OrderedMap) Get(key string) (string, bool) {
	for _, kv := range m {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return "", false
}

// Jobs is the ordered list of jobs; order follows the YAML file.
type Jobs []*Job

// UnmarshalYAML implements yaml.Unmarshaler.
func (j *Jobs) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: jobs must be a mapping", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		job := &Job{}
		if err := n.Content[i+1].Decode(job); err != nil {
			return fmt.Errorf("job %q: %w", n.Content[i].Value, err)
		}
		job.ID = n.Content[i].Value
		*j = append(*j, job)
	}
	return nil
}

// Input is a `workflow_dispatch` input.
type Input struct {
	Name        string
	Description string   `yaml:"description"`
	Required    bool     `yaml:"required"`
	Default     string   `yaml:"default"`
	Type        string   `yaml:"type"`
	Options     []string `yaml:"options"`
}

// Triggers is the normalised `on` section.
type Triggers struct {
	// Events lists every trigger in source order (push, pull_request, ...).
	Events []string
	// Schedules holds the cron expressions from `schedule`.
	Schedules []string
	// Inputs holds the `workflow_dispatch` inputs.
	Inputs []Input
	// Branches maps an event (push / pull_request) to its branch filters.
	Branches map[string][]string
}

// Has reports whether the workflow is triggered by event.
func (t Triggers) Has(event string) bool {
	for _, e := range t.Events {
		if e == event {
			return true
		}
	}
	return false
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (t *Triggers) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode, yaml.SequenceNode:
		var l StringList
		if err := l.UnmarshalYAML(n); err != nil {
			return err
		}
		t.Events = l
		return nil
	case yaml.MappingNode:
	default:
		return fmt.Errorf("line %d: unsupported 'on' value", n.Line)
	}
	t.Branches = map[string][]string{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		event, cfg := n.Content[i].Value, n.Content[i+1]
		t.Events = append(t.Events, event)
		switch event {
		case "schedule":
			var entries []struct {
				Cron string `yaml:"cron"`
			}
			if err := cfg.Decode(&entries); err != nil {
				return fmt.Errorf("on.schedule: %w", err)
			}
			for _, e := range entries {
				t.Schedules = append(t.Schedules, e.Cron)
			}
		case "workflow_dispatch":
			inputs, err := decodeInputs(cfg)
			if err != nil {
				return fmt.Errorf("on.workflow_dispatch: %w", err)
			}
			t.Inputs = inputs
		case "push", "pull_request", "pull_request_target":
			if cfg.Kind != yaml.MappingNode {
				continue
			}
			var f struct {
				Branches StringList `yaml:"branches"`
			}
			if err := cfg.Decode(&f); err != nil {
				return fmt.Errorf("on.%s: %w", event, err)
			}
			if len(f.Branches) > 0 {
				t.Branches[event] = f.Branches
			}
		}
	}
	return nil
}

func decodeInputs(cfg *yaml.Node) ([]Input, error) {
	if cfg.Kind != yaml.MappingNode {
		return nil, nil
	}
	var inputsNode *yaml.Node
	for i := 0; i+1 < len(cfg.Content); i += 2 {
		if cfg.Content[i].Value == "inputs" {
			inputsNode = cfg.Content[i+1]
		}
	}
	if inputsNode == nil || inputsNode.Kind != yaml.MappingNode {
		return nil, nil
	}
	var out []Input
	for i := 0; i+1 < len(inputsNode.Content); i += 2 {
		var in Input
		if err := inputsNode.Content[i+1].Decode(&in); err != nil {
			return nil, fmt.Errorf("input %q: %w", inputsNode.Content[i].Value, err)
		}
		in.Name = inputsNode.Content[i].Value
		out = append(out, in)
	}
	return out, nil
}

// Parse reads a workflow from r.
func Parse(r io.Reader) (*Workflow, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// ParseBytes parses a workflow from YAML bytes.
func ParseBytes(data []byte) (*Workflow, error) {
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse workflow: %w", err)
	}
	if len(wf.Jobs) == 0 {
		return nil, fmt.Errorf("parse workflow: no jobs defined")
	}
	ids := map[string]bool{}
	for _, j := range wf.Jobs {
		ids[j.ID] = true
	}
	for _, j := range wf.Jobs {
		for _, need := range j.Needs {
			if !ids[need] {
				return nil, fmt.Errorf("job %q needs unknown job %q", j.ID, need)
			}
		}
		if j.Uses == "" && len(j.Steps) == 0 {
			return nil, fmt.Errorf("job %q has no steps", j.ID)
		}
	}
	return &wf, nil
}
