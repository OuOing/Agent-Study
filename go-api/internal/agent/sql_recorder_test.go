package agent

import "testing"

func TestMarshalNullableJSON(t *testing.T) {
	value, err := marshalNullableJSON(map[string]any{
		"tool_name": "search_notes",
		"count":     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if value == nil {
		t.Fatal("expected JSON value")
	}
	if value != `{"count":2,"tool_name":"search_notes"}` {
		t.Fatalf("unexpected JSON: %v", value)
	}
}

func TestMarshalNullableJSONReturnsNilForNilMap(t *testing.T) {
	value, err := marshalNullableJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Fatalf("expected nil, got %v", value)
	}
}

func TestNewRunEventID(t *testing.T) {
	id, err := newRunEventID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != len("run_event_")+32 {
		t.Fatalf("unexpected id: %s", id)
	}
}
