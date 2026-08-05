package agent

import "testing"

func TestParseDecisionToolCall(t *testing.T) {
	decision, err := ParseDecision([]byte(`{
		"type":"tool_call",
		"tool_name":"search_notes",
		"arguments":{"query":"会议记录"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Type != "tool_call" || decision.ToolName != "search_notes" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestParseDecisionRejectsUnknownField(t *testing.T) {
	_, err := ParseDecision([]byte(`{"type":"final","content":"done","admin":true}`))
	if err == nil {
		t.Fatal("expected unknown field error")
	}
}

func TestParseDecisionRequiresFinalContent(t *testing.T) {
	_, err := ParseDecision([]byte(`{"type":"final"}`))
	if err == nil {
		t.Fatal("expected missing content error")
	}
}
