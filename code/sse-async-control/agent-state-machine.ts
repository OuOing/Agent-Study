type EventBase = {
  runId: string;
  sequence: number;
  occurredAt: string;
};

export type AgentRunEvent = EventBase & (
  | { type: "run_started" }
  | { type: "status"; step: string }
  | { type: "token"; text: string }
  | { type: "tool_call"; callId: string; toolName: string; input: unknown }
  | { type: "approval_required"; approvalId: string; callId: string }
  | { type: "approval_resolved"; approvalId: string; approved: boolean; reviewedBy: string }
  | { type: "tool_result"; callId: string; output: unknown }
  | { type: "done"; outputTokens: number }
  | { type: "error"; code: string; message: string; retryable: boolean }
  | { type: "cancelled"; reason: string }
);

type ToolState = {
  callId: string;
  toolName: string;
  status: "requested" | "waiting_approval" | "running" | "completed";
  input: unknown;
  output?: unknown;
};

export type AgentRunState = {
  runId?: string;
  phase: "idle" | "running" | "waiting_approval" | "completed" | "failed" | "cancelled";
  lastSequence: number;
  step?: string;
  text: string;
  tools: Record<string, ToolState>;
  pendingApproval?: { approvalId: string; callId: string };
  outputTokens?: number;
  error?: { code: string; message: string; retryable: boolean };
};

export const initialRunState: AgentRunState = {
  phase: "idle",
  lastSequence: 0,
  text: "",
  tools: {},
};

const terminalPhases = new Set<AgentRunState["phase"]>([
  "completed",
  "failed",
  "cancelled",
]);

function illegal(message: string): never {
  throw new Error(`illegal run transition: ${message}`);
}

export function applyRunEvent(
  state: AgentRunState,
  event: AgentRunEvent,
): AgentRunState {
  if (state.runId && state.runId !== event.runId) {
    illegal(`event for ${event.runId} cannot update ${state.runId}`);
  }
  if (event.sequence <= state.lastSequence) return state;
  if (event.sequence !== state.lastSequence + 1) {
    throw new Error(`event gap: expected ${state.lastSequence + 1}, got ${event.sequence}`);
  }
  if (terminalPhases.has(state.phase)) illegal(`${state.phase} is terminal`);

  const next = { ...state, runId: event.runId, lastSequence: event.sequence };
  switch (event.type) {
    case "run_started":
      if (state.phase !== "idle") illegal("run_started requires idle");
      return { ...next, phase: "running" };
    case "status":
      if (state.phase !== "running") illegal("status requires running");
      return { ...next, step: event.step };
    case "token":
      if (state.phase !== "running") illegal("token requires running");
      return { ...next, text: state.text + event.text };
    case "tool_call":
      if (state.phase !== "running") illegal("tool_call requires running");
      if (state.tools[event.callId]) illegal(`duplicate tool call ${event.callId}`);
      return {
        ...next,
        tools: {
          ...state.tools,
          [event.callId]: {
            callId: event.callId,
            toolName: event.toolName,
            status: "requested",
            input: event.input,
          },
        },
      };
    case "approval_required": {
      if (state.phase !== "running") illegal("approval_required requires running");
      const tool = state.tools[event.callId];
      if (!tool) illegal(`unknown tool call ${event.callId}`);
      return {
        ...next,
        phase: "waiting_approval",
        pendingApproval: { approvalId: event.approvalId, callId: event.callId },
        tools: {
          ...state.tools,
          [event.callId]: { ...tool, status: "waiting_approval" },
        },
      };
    }
    case "approval_resolved": {
      if (state.phase !== "waiting_approval" || !state.pendingApproval) {
        illegal("approval_resolved requires a pending approval");
      }
      if (state.pendingApproval.approvalId !== event.approvalId) {
        illegal(`approval ${event.approvalId} is not pending`);
      }
      const callId = state.pendingApproval.callId;
      const tool = state.tools[callId];
      return event.approved
        ? {
            ...next,
            phase: "running",
            pendingApproval: undefined,
            tools: { ...state.tools, [callId]: { ...tool, status: "running" } },
          }
        : { ...next, phase: "cancelled", pendingApproval: undefined };
    }
    case "tool_result": {
      if (state.phase !== "running") illegal("tool_result requires running");
      const tool = state.tools[event.callId];
      if (!tool) illegal(`unknown tool call ${event.callId}`);
      return {
        ...next,
        tools: {
          ...state.tools,
          [event.callId]: { ...tool, status: "completed", output: event.output },
        },
      };
    }
    case "done":
      if (state.phase !== "running") illegal("done requires running");
      return { ...next, phase: "completed", outputTokens: event.outputTokens };
    case "error":
      return {
        ...next,
        phase: "failed",
        error: { code: event.code, message: event.message, retryable: event.retryable },
      };
    case "cancelled":
      return { ...next, phase: "cancelled" };
  }
}

export function makeEvent<T extends Omit<AgentRunEvent, keyof EventBase>>(
  runId: string,
  sequence: number,
  event: T,
): AgentRunEvent {
  return {
    ...event,
    runId,
    sequence,
    occurredAt: new Date(0).toISOString(),
  } as AgentRunEvent;
}
