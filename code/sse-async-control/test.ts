import assert from "node:assert/strict";
import { test } from "node:test";
import { buildServer, retry } from "./server.ts";
import { parseSse, reduceAgentEvent, streamAgent, type AgentViewState } from "./client.ts";
import { InMemoryRunEventLog, toSse } from "./event-log.ts";
import { applyRunEvent, initialRunState, makeEvent } from "./agent-state-machine.ts";

test("SSE endpoint emits status, token and done events", async (t) => {
  const server = buildServer().listen(0);
  t.after(() => server.close());
  await new Promise<void>((resolve) => server.once("listening", resolve));
  const address = server.address();
  assert(address && typeof address === "object");

  const response = await fetch(`http://127.0.0.1:${address.port}/agent/stream`);
  assert.match(response.headers.get("content-type") ?? "", /^text\/event-stream/);
  const body = await response.text();
  assert.match(body, /event: status/);
  assert.equal((body.match(/event: token/g) ?? []).length, 4);
  assert.match(body, /event: done/);
});

test("retry retries transient failures and returns the result", async () => {
  const attempts: number[] = [];
  const result = await retry(async (attempt) => {
    attempts.push(attempt);
    if (attempt < 3) throw new Error("temporary failure");
    return "ok";
  }, { attempts: 3, baseDelayMs: 1 });

  assert.equal(result, "ok");
  assert.deepEqual(attempts, [1, 2, 3]);
});

test("retry respects caller cancellation", async () => {
  const controller = new AbortController();
  controller.abort(new Error("cancelled by user"));
  await assert.rejects(
    retry(async (_attempt, signal) => {
      signal.throwIfAborted();
      return "unreachable";
    }, { attempts: 3, baseDelayMs: 1, signal: controller.signal }),
    /cancelled by user/,
  );
});

test("SSE parser handles arbitrary boundaries and UTF-8 characters", async () => {
  const bytes = new TextEncoder().encode(
    'event: token\r\ndata: {"type":"token","text":"你好"}\r\n\r\n' +
    'event: done\ndata: {"type":"done","usage":{"outputTokens":1}}\n\n',
  );
  const cuts = [0, 7, 29, 45, bytes.length];
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (let index = 1; index < cuts.length; index += 1) {
        controller.enqueue(bytes.slice(cuts[index - 1], cuts[index]));
      }
      controller.close();
    },
  });
  const frames = [];
  for await (const frame of parseSse(body)) frames.push(frame);
  assert.deepEqual(frames, [
    { event: "token", data: '{"type":"token","text":"你好"}' },
    { event: "done", data: '{"type":"done","usage":{"outputTokens":1}}' },
  ]);
});

test("streamed events reduce into a completed view state", async (t) => {
  const server = buildServer().listen(0);
  t.after(() => server.close());
  await new Promise<void>((resolve) => server.once("listening", resolve));
  const address = server.address();
  assert(address && typeof address === "object");
  let state: AgentViewState = { status: "idle", text: "" };
  await streamAgent(`http://127.0.0.1:${address.port}/agent/stream`, {
    onEvent(event) { state = reduceAgentEvent(state, event); },
  });
  assert.deepEqual(state, {
    status: "completed",
    step: "thinking",
    text: "Agent 正在流式回答。",
    outputTokens: 4,
  });
});

test("event log assigns monotonic sequences and replays only missing events", async () => {
  const log = new InMemoryRunEventLog();
  const first = log.append("run-1", "status", { step: "thinking" });
  const second = log.append("run-1", "token", { text: "Agent" });
  const third = log.append("run-1", "token", { text: " 正在" });

  assert.deepEqual([first.sequence, second.sequence, third.sequence], [1, 2, 3]);
  assert.deepEqual(log.replayAfter("run-1", 1).map((event) => event.sequence), [2, 3]);
  assert.deepEqual(log.replayAfter("another-run", 0), []);
});

test("SSE id survives serialization and arbitrary stream chunks", async () => {
  const log = new InMemoryRunEventLog();
  const serialized = toSse(log.append("run-1", "token", { text: "你好" }));
  const bytes = new TextEncoder().encode(serialized);
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(bytes.slice(0, 5));
      controller.enqueue(bytes.slice(5));
      controller.close();
    },
  });
  const frames = [];
  for await (const frame of parseSse(body)) frames.push(frame);
  assert.equal(frames.length, 1);
  assert.equal(frames[0].id, "1");
  assert.equal(frames[0].event, "token");
  assert.equal(JSON.parse(frames[0].data).runId, "run-1");
});

test("approval pauses a tool and approval resumes it before completion", () => {
  const events = [
    makeEvent("run-approval", 1, { type: "run_started" }),
    makeEvent("run-approval", 2, {
      type: "tool_call", callId: "call-1", toolName: "send_email", input: { to: "boss@example.com" },
    }),
    makeEvent("run-approval", 3, {
      type: "approval_required", approvalId: "approval-1", callId: "call-1",
    }),
    makeEvent("run-approval", 4, {
      type: "approval_resolved", approvalId: "approval-1", approved: true, reviewedBy: "user-1",
    }),
    makeEvent("run-approval", 5, {
      type: "tool_result", callId: "call-1", output: { messageId: "message-1" },
    }),
    makeEvent("run-approval", 6, { type: "token", text: "邮件已发送。" }),
    makeEvent("run-approval", 7, { type: "done", outputTokens: 8 }),
  ];
  const state = events.reduce(applyRunEvent, initialRunState);
  assert.equal(state.phase, "completed");
  assert.equal(state.text, "邮件已发送。");
  assert.equal(state.tools["call-1"].status, "completed");
  assert.deepEqual(state.tools["call-1"].output, { messageId: "message-1" });
});

test("duplicate events are ignored and gaps are rejected", () => {
  const started = makeEvent("run-sequence", 1, { type: "run_started" });
  const running = applyRunEvent(initialRunState, started);
  assert.equal(applyRunEvent(running, started), running);
  assert.throws(
    () => applyRunEvent(running, makeEvent("run-sequence", 3, { type: "token", text: "gap" })),
    /event gap: expected 2, got 3/,
  );
});

test("rejected approval becomes terminal and cannot receive a tool result", () => {
  const beforeRejection = [
    makeEvent("run-rejected", 1, { type: "run_started" }),
    makeEvent("run-rejected", 2, { type: "tool_call", callId: "call-1", toolName: "delete_file", input: {} }),
    makeEvent("run-rejected", 3, { type: "approval_required", approvalId: "approval-1", callId: "call-1" }),
  ].reduce(applyRunEvent, initialRunState);
  const rejected = applyRunEvent(beforeRejection, makeEvent("run-rejected", 4, {
    type: "approval_resolved", approvalId: "approval-1", approved: false, reviewedBy: "user-1",
  }));
  assert.equal(rejected.phase, "cancelled");
  assert.throws(
    () => applyRunEvent(rejected, makeEvent("run-rejected", 5, {
      type: "tool_result", callId: "call-1", output: "should not happen",
    })),
    /cancelled is terminal/,
  );
});

test("error closes a waiting approval with a stable error model", () => {
  const waiting = [
    makeEvent("run-error", 1, { type: "run_started" }),
    makeEvent("run-error", 2, { type: "tool_call", callId: "call-1", toolName: "deploy", input: {} }),
    makeEvent("run-error", 3, { type: "approval_required", approvalId: "approval-1", callId: "call-1" }),
  ].reduce(applyRunEvent, initialRunState);
  const failed = applyRunEvent(waiting, makeEvent("run-error", 4, {
    type: "error", code: "APPROVAL_EXPIRED", message: "审批已过期", retryable: false,
  }));
  assert.equal(failed.phase, "failed");
  assert.deepEqual(failed.error, {
    code: "APPROVAL_EXPIRED", message: "审批已过期", retryable: false,
  });
});
