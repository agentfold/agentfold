package domain

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// HasOutputSchema reports whether the agent must submit a structured output.
func (c AgentConfig) HasOutputSchema() bool {
	return len(c.OutputSchema) > 0 && string(c.OutputSchema) != "null"
}

// schemaURL names the schema being compiled; it only shows in errors.
const schemaURL = "output_schema.json"

func compileSchema(schema json.RawMessage) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	_ = c.AddResource(schemaURL, doc) // fails only for a URL already added, and the compiler is new
	return c.Compile(schemaURL)
}

// ValidateOutput checks a structured output against the schema of its node.
func ValidateOutput(schema, output json.RawMessage) error {
	compiled, err := compileSchema(schema)
	if err != nil {
		return fmt.Errorf("the node output schema is invalid: %w", err)
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(output))
	if err != nil {
		return fmt.Errorf("the output is not JSON: %w", err)
	}
	return compiled.Validate(value)
}
