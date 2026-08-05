package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type wireDecision struct {
	Type      string          `json:"type"`
	ToolName  string          `json:"tool_name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Content   string          `json:"content,omitempty"`
}

func ParseDecision(data []byte) (Decision, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var wire wireDecision
	if err := decoder.Decode(&wire); err != nil {
		return Decision{}, fmt.Errorf("decode decision: %w", err)
	}

	switch wire.Type {
	case "final":
		if wire.Content == "" {
			return Decision{}, errors.New("final content is required")
		}
		return Decision{Type: "final", Content: wire.Content}, nil

	case "tool_call":
		if wire.ToolName == "" {
			return Decision{}, errors.New("tool_name is required")
		}
		arguments := map[string]any{}
		if len(wire.Arguments) == 0 {
			return Decision{}, errors.New("arguments are required")
		}
		if err := json.Unmarshal(wire.Arguments, &arguments); err != nil {
			return Decision{}, fmt.Errorf("decode arguments: %w", err)
		}
		return Decision{Type: "tool_call", ToolName: wire.ToolName, Arguments: arguments}, nil

	default:
		return Decision{}, fmt.Errorf("unsupported decision type: %q", wire.Type)
	}
}
