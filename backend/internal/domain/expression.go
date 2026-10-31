package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/file"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// Expressions are written in expr-lang (https://expr-lang.org): pure, with
// no I/O, bounded in size and memory. They read three variables:
//   - card: id, title, description, lane_id, card_type_id, project_id,
//     parent_card_id and properties;
//   - input: what the node before handed on;
//   - nodes: the last output of each node of the run that ran, by node id
//     (nodes.dev, or nodes["agent-1"] for ids with dashes).
//
// An output is its structured output when it has one, else its result
// text, else the whole JSON value.

// maxExpressionNodes bounds the size of an expression.
const maxExpressionNodes = 1000

// exprEnv declares the variables, so unknown names fail to compile.
var exprEnv = map[string]any{"card": map[string]any{}, "input": nil, "nodes": map[string]any{}}

func compileExpression(src string) (*vm.Program, error) {
	program, err := expr.Compile(src, expr.Env(exprEnv), expr.MaxNodes(maxExpressionNodes))
	return program, exprError(err)
}

// CheckExpression reports why src does not compile.
func CheckExpression(src string) error {
	_, err := compileExpression(src)
	return err
}

// Evaluate runs the expression src on data.
func Evaluate(src string, data TemplateData) (any, error) {
	program, err := compileExpression(src)
	if err != nil {
		return nil, err
	}
	out, err := expr.Run(program, data.env())
	return out, exprError(err)
}

// exprError keeps the message of an expr error, without the source
// snippet it adds.
func exprError(err error) error {
	var fe *file.Error
	if errors.As(err, &fe) {
		return errors.New(fe.Message)
	}
	return err
}

// ExpressionReferences lists the ids of the nodes src reads.
func ExpressionReferences(src string) []string {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil
	}
	refs := &nodeRefs{}
	ast.Walk(&tree.Node, refs)
	return refs.ids
}

type nodeRefs struct{ ids []string }

func (r *nodeRefs) Visit(node *ast.Node) {
	member, ok := (*node).(*ast.MemberNode)
	if !ok {
		return
	}
	root, ok := member.Node.(*ast.IdentifierNode)
	if !ok || root.Value != "nodes" {
		return
	}
	if id, ok := member.Property.(*ast.StringNode); ok {
		r.ids = append(r.ids, id.Value)
	}
}

// env is what expressions read.
func (d TemplateData) env() map[string]any {
	card := map[string]any{}
	for name, field := range cardFields {
		card[name] = field(d.Card)
	}
	card["properties"] = maps.Clone(d.Card.Properties)
	nodes := map[string]any{}
	for id, output := range d.Nodes {
		nodes[id] = outputData(output)
	}
	return map[string]any{"card": card, "input": outputData(d.Input), "nodes": nodes}
}

// outputData decodes an output: its structured output, else its result
// text, else the whole value; nil when empty or not JSON.
func outputData(output json.RawMessage) any {
	var v any
	if json.Unmarshal(output, &v) != nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		if m["output"] != nil {
			return m["output"]
		}
		if result, ok := m["result"].(string); ok {
			return result
		}
	}
	return v
}

// TransformConfig builds the output of a Transform node from Output, a
// JSON value: strings starting with = are expressions, whose values keep
// their type; other strings are text with placeholders (see
// RenderTemplate).
type TransformConfig struct {
	Output json.RawMessage `json:"output,omitempty"`
}

// Apply builds the output on data, as {"output": <value>}.
func (c TransformConfig) Apply(data TemplateData) (json.RawMessage, error) {
	var template any
	if err := json.Unmarshal(c.Output, &template); err != nil {
		return nil, fmt.Errorf("configuração inválida: %w", err)
	}
	value, err := build(template, "", data)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(map[string]any{"output": value})
	if err != nil {
		return nil, fmt.Errorf("o resultado não vira JSON: %w", err)
	}
	return out, nil
}

// build fills in a template value; path names it in errors.
func build(v any, path string, data TemplateData) (any, error) {
	switch t := v.(type) {
	case string:
		src, ok := strings.CutPrefix(t, "=")
		if !ok {
			return RenderTemplate(t, data, EscapeNone), nil
		}
		out, err := Evaluate(src, data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", where(path), err)
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			var err error
			if out[k], err = build(item, join(path, k), data); err != nil {
				return nil, err
			}
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			var err error
			if out[i], err = build(item, join(path, fmt.Sprint(i)), data); err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return v, nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func where(path string) string {
	if path == "" {
		return "a expressão"
	}
	return "a expressão em " + path
}

// templateStrings lists the strings of a template value with their paths.
func templateStrings(v any, path string, visit func(path, s string)) {
	switch t := v.(type) {
	case string:
		visit(path, t)
	case map[string]any:
		for k, item := range t {
			templateStrings(item, join(path, k), visit)
		}
	case []any:
		for i, item := range t {
			templateStrings(item, join(path, fmt.Sprint(i)), visit)
		}
	}
}

func transformProblems(c TransformConfig) []string {
	var template any
	if len(c.Output) == 0 || json.Unmarshal(c.Output, &template) != nil || template == nil {
		return []string{"monte a saída do nó em JSON"}
	}
	var out []string
	templateStrings(template, "", func(path, s string) {
		if src, ok := strings.CutPrefix(s, "="); ok {
			if err := CheckExpression(src); err != nil {
				out = append(out, fmt.Sprintf("%s é inválida: %s", where(path), err))
			}
		} else if err := checkPlaceholders(s); err != nil {
			out = append(out, err.Error())
		}
	})
	slices.Sort(out)
	return out
}
