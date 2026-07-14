/**
 * @template T
 * @param {string} buffer
 * @returns {{ events: T[], remaining: string }}
 */
export function parseSSE(buffer) {
  const messages = buffer.split("\n\n");
  const remaining = messages.pop() ?? "";
  const events = [];
  for (const message of messages) {
    const data = message.split("\n").find((line) => line.startsWith("data: "))?.slice(6);
    if (!data) continue;
    try {
      events.push(JSON.parse(data));
    } catch {
      // A malformed event is isolated; subsequent complete events can still be processed.
    }
  }
  return { events, remaining };
}
