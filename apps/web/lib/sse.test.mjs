import assert from "node:assert/strict";
import test from "node:test";
import { parseSSE } from "./sse.mjs";

test("parseSSE preserves partial chunks and skips malformed events", () => {
  const first = parseSSE("event: status\ndata: [1]\n\nevent: status\ndata: [2");
  assert.deepEqual(first.events, [[1]]);
  assert.equal(first.remaining, "event: status\ndata: [2");

  const second = parseSSE(first.remaining + "]\n\ndata: invalid\n\ndata: [3]\n\n");
  assert.deepEqual(second.events, [[2], [3]]);
  assert.equal(second.remaining, "");
});
