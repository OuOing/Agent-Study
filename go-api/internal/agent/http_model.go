package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const maxModelResponseBytes = 1 << 20

type HTTPModel struct {
	client   *http.Client
	endpoint string
	apiKey   string
	model    string
	tools    []ToolDefinition
}

func NewHTTPModel(client *http.Client, endpoint, apiKey, model string, tools []ToolDefinition) *HTTPModel {
	return &HTTPModel{client: client, endpoint: endpoint, apiKey: apiKey, model: model, tools: tools}
}

type modelRequest struct {
	Model       string           `json:"model"`
	Goal        string           `json:"goal"`
	Observation map[string]any   `json:"observation,omitempty"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
}

type modelResponse struct {
	Decision json.RawMessage `json:"decision"`
}

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("model API returned status %d", e.StatusCode)
}

func (m *HTTPModel) Decide(ctx context.Context, goal string, observation map[string]any) (Decision, error) {
	payload, err := json.Marshal(modelRequest{
		Model:       m.model,
		Goal:        goal,
		Observation: observation,
		Tools:       m.tools,
	})
	if err != nil {
		return Decision{}, fmt.Errorf("encode model request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(payload))
	if err != nil {
		return Decision{}, fmt.Errorf("create model request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("call model API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxModelResponseBytes))
	if err != nil {
		return Decision{}, fmt.Errorf("read model response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Decision{}, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var decoded modelResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Decision{}, fmt.Errorf("decode model envelope: %w", err)
	}
	return ParseDecision(decoded.Decision)
}
