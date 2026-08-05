package agent

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func SearchNotesDefinition() ToolDefinition {
	return ToolDefinition{
		Name:        "search_notes",
		Description: "Search meeting notes for information related to the query.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search phrase derived from the user's goal.",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
	}
}
