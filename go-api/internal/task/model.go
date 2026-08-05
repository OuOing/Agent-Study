package task

type Task struct {
	ID     string `json:"id"`
	UserID string `json:"-"`
	Goal   string `json:"goal"`
	Status string `json:"status"`
}

type CreateInput struct {
	Goal string `json:"goal"`
}
