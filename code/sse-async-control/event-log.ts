export type RunEvent<TPayload = unknown> = {
  runId: string;
  sequence: number;
  occurredAt: string;
  type: string;
  payload: TPayload;
};

export class InMemoryRunEventLog {
  private readonly eventsByRun = new Map<string, RunEvent[]>();

  append<TPayload>(
    runId: string,
    type: string,
    payload: TPayload,
  ): RunEvent<TPayload> {
    const events = this.eventsByRun.get(runId) ?? [];
    const event: RunEvent<TPayload> = {
      runId,
      sequence: (events.at(-1)?.sequence ?? 0) + 1,
      occurredAt: new Date().toISOString(),
      type,
      payload,
    };
    events.push(event);
    this.eventsByRun.set(runId, events);
    return event;
  }

  replayAfter(runId: string, lastSequence = 0): readonly RunEvent[] {
    return (this.eventsByRun.get(runId) ?? []).filter(
      (event) => event.sequence > lastSequence,
    );
  }
}

export function toSse(event: RunEvent): string {
  return [
    `id: ${event.sequence}`,
    `event: ${event.type}`,
    `data: ${JSON.stringify(event)}`,
    "",
    "",
  ].join("\n");
}
