package task

type Status string

const (
	StatusCreated   Status = "created"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Task struct {
	ID        string      `json:"id"`
	UserID    string      `json:"-"`
	Goal      string      `json:"goal"`
	Status    Status      `json:"status"`
	Result    *TaskResult `json:"result,omitempty"`
	ErrorCode string      `json:"error_code,omitempty"`
}

type TaskResult struct {
	Content string `json:"content"`
}

type CreateInput struct {
	Goal string `json:"goal"`
}
