package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// TemplateData is what placeholders read: the card, what the node before
// handed on and the last output of each node of the run that ran, by node
// id.
type TemplateData struct {
	Card  Card
	Input json.RawMessage
	Nodes map[string]json.RawMessage
}

var placeholder = regexp.MustCompile(`\{\{\s*([^{}\s]*)\s*\}\}`)

// cardFields are the card fields placeholders read, by name.
var cardFields = map[string]func(Card) string{
	"id":             func(c Card) string { return c.ID.String() },
	"title":          func(c Card) string { return c.Title },
	"description":    func(c Card) string { return c.Description },
	"lane_id":        func(c Card) string { return c.LaneID.String() },
	"card_type_id":   func(c Card) string { return c.CardTypeID.String() },
	"project_id":     func(c Card) string { return optionalID(c.ProjectID) },
	"parent_card_id": func(c Card) string { return optionalID(c.ParentCardID) },
}

func optionalID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// checkPlaceholders reports the first placeholder that names nothing.
func checkPlaceholders(text string) error {
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		if !knownPlaceholder(m[1]) {
			return fmt.Errorf("placeholder desconhecido: %s (use card.<campo>, card.properties.<chave>, input, input.<caminho>, nodes.<id> ou nodes.<id>.<caminho>)", m[0])
		}
	}
	return nil
}

func knownPlaceholder(path string) bool {
	root, rest, _ := strings.Cut(path, ".")
	switch root {
	case "input":
		return true
	case "nodes":
		id, _, _ := strings.Cut(rest, ".")
		return id != ""
	case "card":
		if key, ok := strings.CutPrefix(rest, "properties."); ok {
			return key != ""
		}
		_, ok := cardFields[rest]
		return ok
	}
	return false
}

// HasPlaceholders tells whether text has placeholders to fill in.
func HasPlaceholders(text string) bool { return placeholder.MatchString(text) }

// NodeReferences lists the ids of the nodes the placeholders of text read,
// in order, repeated as often as they appear.
func NodeReferences(text string) []string {
	var out []string
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		if rest, ok := strings.CutPrefix(m[1], "nodes."); ok && knownPlaceholder(m[1]) {
			id, _, _ := strings.Cut(rest, ".")
			out = append(out, id)
		}
	}
	return out
}

// RenderTemplate replaces the placeholders of text:
//   - {{card.<field>}}: id, title, description, lane_id, card_type_id,
//     project_id or parent_card_id of the card;
//   - {{card.properties.<key>}}: a property of the card;
//   - {{input}}: what the node before handed on (see MessageFrom);
//   - {{input.<path>}}: a value in it, by keys and array indexes split by
//     dots, inside its structured output when it has one;
//   - {{nodes.<id>}} and {{nodes.<id>.<path>}}: the same for the last
//     output of the node id in the run.
//
// escape encodes each value for where it goes; values that are not
// strings come as JSON, with text false. Missing values become empty
// strings; unknown placeholders stay as they are.
func RenderTemplate(text string, data TemplateData, escape func(value string, text bool) string) string {
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		path := placeholder.FindStringSubmatch(m)[1]
		if !knownPlaceholder(path) {
			return m
		}
		return escape(lookup(path, data))
	})
}

// lookup reads a known placeholder.
func lookup(path string, data TemplateData) (string, bool) {
	root, rest, _ := strings.Cut(path, ".")
	switch root {
	case "input":
		return outputValue(data.Input, rest)
	case "nodes":
		id, rest, _ := strings.Cut(rest, ".")
		return outputValue(data.Nodes[id], rest)
	}
	if key, ok := strings.CutPrefix(rest, "properties."); ok {
		return data.Card.Properties[key], true
	}
	return cardFields[rest](data.Card), true
}

// outputValue is the message of an output for an empty path, else the
// value at path in it.
func outputValue(output json.RawMessage, path string) (string, bool) {
	if path == "" {
		return MessageFrom(output), true
	}
	return inputValue(output, strings.Split(path, "."))
}

// inputValue walks path in the input, inside its output when it has one.
func inputValue(input json.RawMessage, path []string) (string, bool) {
	var v any
	if json.Unmarshal(input, &v) != nil {
		return "", true
	}
	if m, ok := v.(map[string]any); ok && m["output"] != nil {
		v = m["output"]
	}
	for _, key := range path {
		switch node := v.(type) {
		case map[string]any:
			v = node[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(node) {
				return "", true
			}
			v = node[i]
		default:
			return "", true
		}
	}
	switch value := v.(type) {
	case nil:
		return "", true
	case string:
		return value, true
	default:
		b, _ := json.Marshal(value) // decoded from JSON: marshals
		return string(b), false
	}
}

// EscapeNone leaves values as they are, for text read by agents.
func EscapeNone(s string, _ bool) string { return s }
