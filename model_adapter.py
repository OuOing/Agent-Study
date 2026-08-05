"""Model boundary used by the workflow.

The demo implementation is deterministic. A real adapter would use an SDK
and MODEL_API_KEY from the environment, while keeping this return shape.
"""

from config import MODEL_API_KEY, MODEL_NAME


def decide(goal: str, tool_result=None) -> dict:
    """Return a normalized decision independent of a model vendor."""
    if tool_result is None:
        return {
            "type": "tool_call",
            "name": "search_meeting_notes",
            "arguments": {"query": goal},
            "model": MODEL_NAME,
            "configured": bool(MODEL_API_KEY),
        }
    return {"type": "final", "content": "已根据会议记录生成待办草稿，等待确认后写入。"}

