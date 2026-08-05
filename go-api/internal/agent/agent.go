package agent

import (
	"context"
	"errors"
	"time"
)

var ErrMaxSteps = errors.New("agent reached max steps")

type Decision struct {
	Type      string
	ToolName  string
	Arguments map[string]any
	Content   string
}

type Model interface {
	Decide(ctx context.Context, goal string, observation map[string]any) (Decision, error)
}

type Tool interface {
	Name() string
	Execute(ctx context.Context, arguments map[string]any) (map[string]any, error)
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, tool := range tools {
		r.tools[tool.Name()] = tool
	}
	return r
}

func (r *Registry) Execute(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	tool, ok := r.tools[name]
	if !ok {
		return nil, errors.New("unknown tool: " + name)
	}
	return tool.Execute(ctx, arguments)
}

type Orchestrator struct {
	model    Model
	registry *Registry
	maxSteps int
	recorder RunRecorder
}

func NewOrchestrator(model Model, registry *Registry, maxSteps int) *Orchestrator {
	return NewOrchestratorWithRecorder(model, registry, maxSteps, NoopRecorder{})
}

func NewOrchestratorWithRecorder(model Model, registry *Registry, maxSteps int, recorder RunRecorder) *Orchestrator {
	if recorder == nil {
		recorder = NoopRecorder{}
	}
	return &Orchestrator{model: model, registry: registry, maxSteps: maxSteps, recorder: recorder}
}

func (o *Orchestrator) Run(ctx context.Context, goal string) (string, error) {
	observation := map[string]any{}
	for step := 0; step < o.maxSteps; step++ {
		modelStartedAt := time.Now()
		decision, err := o.model.Decide(ctx, goal, observation)
		modelEvent := RunEvent{
			Step: step,
			Kind: "model_call",
			Input: map[string]any{
				"has_observation": len(observation) > 0,
			},
			Duration: time.Since(modelStartedAt),
		}
		if err != nil {
			modelEvent.Error = err.Error()
			_ = o.recorder.Record(ctx, modelEvent)
			return "", err
		}
		modelEvent.DecisionType = decision.Type
		modelEvent.Output = map[string]any{
			"decision_type": decision.Type,
			"tool_name":     decision.ToolName,
		}
		_ = o.recorder.Record(ctx, modelEvent)

		if decision.Type == "final" {
			return decision.Content, nil
		}
		if decision.Type != "tool_call" {
			return "", errors.New("invalid decision type")
		}
		toolStartedAt := time.Now()
		result, err := o.registry.Execute(ctx, decision.ToolName, decision.Arguments)
		toolEvent := RunEvent{
			Step:     step,
			Kind:     "tool_call",
			ToolName: decision.ToolName,
			Input:    decision.Arguments,
			Duration: time.Since(toolStartedAt),
		}
		if err != nil {
			toolEvent.Error = err.Error()
			_ = o.recorder.Record(ctx, toolEvent)
			return "", err
		}
		toolEvent.Output = result
		_ = o.recorder.Record(ctx, toolEvent)
		observation = result
	}
	return "", ErrMaxSteps
}

type DemoModel struct{}

func (DemoModel) Decide(_ context.Context, _ string, observation map[string]any) (Decision, error) {
	if len(observation) == 0 {
		return Decision{Type: "tool_call", ToolName: "search_notes", Arguments: map[string]any{"query": "会议记录"}}, nil
	}
	return Decision{Type: "final", Content: "已根据检索结果生成待办草稿。"}, nil
}

type SearchNotesTool struct{}

func (SearchNotesTool) Name() string { return "search_notes" }

func (SearchNotesTool) Execute(_ context.Context, arguments map[string]any) (map[string]any, error) {
	query, ok := arguments["query"].(string)
	if !ok || query == "" {
		return nil, errors.New("query is required")
	}
	return map[string]any{"matches": []string{"会议记录：周五前整理项目风险和负责人"}}, nil
}
