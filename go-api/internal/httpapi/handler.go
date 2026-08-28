package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"agent-go-api/internal/task"
	"agent-go-api/internal/worker"
)

type Handler struct {
	tasks  *task.Service
	worker *worker.Worker
}

func NewHandler(tasks *task.Service, w *worker.Worker) *Handler {
	return &Handler{tasks: tasks, worker: w}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/tasks", h.tasksEndpoint)
	mux.HandleFunc("/tasks/", h.taskByID)
	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) tasksEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	var input task.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	created, err := h.tasks.Create(r.Context(), input)
	if errors.Is(err, task.ErrInvalidGoal) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := h.worker.Submit(r.Context(), worker.Job{ID: created.ID, Goal: created.Goal}); err != nil {
		_ = h.tasks.Fail(r.Context(), created.ID, "queue_unavailable")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "queue_unavailable"})
		return
	}
	writeJSON(w, http.StatusAccepted, created)
}

func (h *Handler) taskByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
		return
	}
	id := r.URL.Path[len("/tasks/"):]
	found, err := h.tasks.Get(r.Context(), id)
	if errors.Is(err, task.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, found)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
