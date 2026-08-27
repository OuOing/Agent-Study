import { createServer, type ServerResponse } from "node:http";

type AgentEvent =
  | { type: "status"; step: string }
  | { type: "token"; text: string }
  | { type: "done"; usage: { outputTokens: number } };

const sleep = (ms: number, signal?: AbortSignal) =>
  new Promise<void>((resolve, reject) => {
    const timer = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      reject(signal.reason);
    }, { once: true });
  });

export async function retry<T>(
  operation: (attempt: number, signal: AbortSignal) => Promise<T>,
  options: { attempts: number; baseDelayMs: number; signal?: AbortSignal },
): Promise<T> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= options.attempts; attempt += 1) {
    const signal = options.signal
      ? AbortSignal.any([options.signal, AbortSignal.timeout(1_000)])
      : AbortSignal.timeout(1_000);
    try {
      return await operation(attempt, signal);
    } catch (error) {
      lastError = error;
      if (options.signal?.aborted || attempt === options.attempts) throw error;
      await sleep(options.baseDelayMs * 2 ** (attempt - 1), options.signal);
    }
  }
  throw lastError;
}

function sendEvent(response: ServerResponse, event: AgentEvent) {
  response.write(`event: ${event.type}\n`);
  response.write(`data: ${JSON.stringify(event)}\n\n`);
}

export function buildServer() {
  return createServer(async (request, response) => {
    if (request.url !== "/agent/stream") {
      response.writeHead(404).end("Not found");
      return;
    }

    response.writeHead(200, {
      "Content-Type": "text/event-stream; charset=utf-8",
      "Cache-Control": "no-cache, no-transform",
      Connection: "keep-alive",
    });

    const disconnected = new AbortController();
    request.on("close", () => disconnected.abort(new Error("client disconnected")));

    try {
      sendEvent(response, { type: "status", step: "thinking" });
      for (const text of ["Agent", " 正在", "流式", "回答。"] ) {
        await sleep(60, disconnected.signal);
        sendEvent(response, { type: "token", text });
      }
      sendEvent(response, { type: "done", usage: { outputTokens: 4 } });
      response.end();
    } catch {
      if (!response.destroyed) response.end();
    }
  });
}

if (process.argv[1]?.endsWith("server.ts")) {
  buildServer().listen(3000, () => {
    console.log("SSE lab: http://localhost:3000/agent/stream");
  });
}
