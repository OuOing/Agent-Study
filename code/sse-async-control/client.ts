export type AgentEvent =
  | { type: "status"; step: string }
  | { type: "token"; text: string }
  | { type: "done"; usage: { outputTokens: number } };

export type SseFrame = { event: string; data: string; id?: string };

function parseFrame(rawFrame: string): SseFrame | undefined {
  let event = "message";
  let id: string | undefined;
  const dataLines: string[] = [];
  for (const line of rawFrame.split("\n")) {
    if (line === "" || line.startsWith(":")) continue;
    const colon = line.indexOf(":");
    const field = colon === -1 ? line : line.slice(0, colon);
    let value = colon === -1 ? "" : line.slice(colon + 1);
    if (value.startsWith(" ")) value = value.slice(1);
    if (field === "event") event = value;
    if (field === "id") id = value;
    if (field === "data") dataLines.push(value);
  }
  if (dataLines.length === 0) return undefined;
  return id === undefined
    ? { event, data: dataLines.join("\n") }
    : { event, data: dataLines.join("\n"), id };
}

export async function* parseSse(body: ReadableStream<Uint8Array>): AsyncGenerator<SseFrame> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done }).replaceAll("\r\n", "\n");
      let boundary = buffer.indexOf("\n\n");
      while (boundary !== -1) {
        const rawFrame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);
        const frame = parseFrame(rawFrame);
        if (frame) yield frame;
        boundary = buffer.indexOf("\n\n");
      }
      if (done) break;
    }
  } finally {
    reader.releaseLock();
  }
}

export async function streamAgent(url: string, options: {
  signal?: AbortSignal;
  onEvent: (event: AgentEvent) => void;
}) {
  const response = await fetch(url, {
    headers: { Accept: "text/event-stream" },
    signal: options.signal,
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  if (!response.body) throw new Error("response body is unavailable");
  for await (const frame of parseSse(response.body)) {
    const event = JSON.parse(frame.data) as AgentEvent;
    if (event.type !== frame.event) {
      throw new Error(`event mismatch: ${frame.event} != ${event.type}`);
    }
    options.onEvent(event);
    if (event.type === "done") return;
  }
  throw new Error("stream ended before a done event");
}

export type AgentViewState = {
  status: "idle" | "running" | "completed";
  step?: string;
  text: string;
  outputTokens?: number;
};

export function reduceAgentEvent(state: AgentViewState, event: AgentEvent): AgentViewState {
  switch (event.type) {
    case "status":
      return { ...state, status: "running", step: event.step };
    case "token":
      return { ...state, status: "running", text: state.text + event.text };
    case "done":
      return { ...state, status: "completed", outputTokens: event.usage.outputTokens };
  }
}
