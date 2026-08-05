"""A dependency-free Agent workflow demo.

Run with: python3 app.py
Then create a task with:
  curl -X POST http://127.0.0.1:8000/tasks \
    -H 'Content-Type: application/json' \
    -d '{"goal":"整理今天的会议并创建待办"}'
"""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import threading
import uuid

from model_adapter import decide


TASKS = {}
LOCK = threading.Lock()


def execute_tool(name: str, arguments: dict) -> dict:
    """Validate and execute a tool. Real tools call databases or APIs here."""
    if name != "search_meeting_notes":
        raise ValueError("unknown_tool")
    query = str(arguments.get("query", "")).strip()
    if not query:
        raise ValueError("query_required")
    return {"matches": ["会议记录示例：周五前整理项目风险和负责人"]}


def run_agent(task_id: str, goal: str) -> None:
    """Run the model -> tool -> observation loop with an approval boundary."""
    try:
        with LOCK:
            TASKS[task_id].update(status="understanding", message="模型正在判断下一步")
        decision = decide(goal)
        if decision["type"] == "tool_call":
            with LOCK:
                TASKS[task_id].update(status="tool_calling", message="正在查询会议记录", tool=decision)
            result = execute_tool(decision["name"], decision["arguments"])
            with LOCK:
                TASKS[task_id].update(status="draft_ready", message="已获得资料，正在生成草稿", tool_result=result)
            final = decide(goal, result)
            with LOCK:
                TASKS[task_id].update(status="awaiting_approval", message=final["content"], result=final)
    except (ValueError, KeyError) as exc:
        with LOCK:
            TASKS[task_id].update(status="failed", message=str(exc), error=str(exc))


class Handler(BaseHTTPRequestHandler):
    def _send(self, code: int, payload: dict) -> None:
        data = json.dumps(payload, ensure_ascii=False).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self) -> None:
        if self.path != "/tasks":
            self._send(404, {"error": "not_found"})
            return
        try:
            length = int(self.headers.get("Content-Length", "0"))
            body = json.loads(self.rfile.read(length) or b"{}")
            goal = str(body["goal"]).strip()
            if not goal:
                raise ValueError("goal must not be empty")
        except (KeyError, ValueError, json.JSONDecodeError) as exc:
            self._send(400, {"error": str(exc)})
            return

        task_id = uuid.uuid4().hex
        with LOCK:
            TASKS[task_id] = {"id": task_id, "goal": goal, "status": "created", "message": "任务已创建"}
        threading.Thread(target=run_agent, args=(task_id, goal), daemon=True).start()
        self._send(202, TASKS[task_id])

    def do_GET(self) -> None:
        prefix = "/tasks/"
        if not self.path.startswith(prefix):
            self._send(404, {"error": "not_found"})
            return
        task_id = self.path[len(prefix):]
        with LOCK:
            task = TASKS.get(task_id)
        if task is None:
            self._send(404, {"error": "task_not_found"})
            return
        self._send(200, task)

    def log_message(self, *_args) -> None:
        return


if __name__ == "__main__":
    server = ThreadingHTTPServer(("127.0.0.1", 8000), Handler)
    print("Agent demo listening at http://127.0.0.1:8000")
    server.serve_forever()
