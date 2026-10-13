import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const source = readFileSync(new URL("../internal/core/resources/PiStatusExtension.js", import.meta.url), "utf8");
const { default: register } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);
const folder = mkdtempSync(join(tmpdir(), "orchestra-pi-"));
const keys = ["ORCHESTRA_STATUS_FILE", "ORCHESTRA_AGENT_ID_FILE"];
const saved = keys.map(key => process.env[key]);
try {
  process.env.ORCHESTRA_STATUS_FILE = join(folder, "status");
  process.env.ORCHESTRA_AGENT_ID_FILE = join(folder, "id");
  const handlers = new Map();
  let id = "conversation-a";
  const ctx = { sessionManager: { getSessionId: () => id } };
  register({ on: (event, handler) => handlers.set(event, handler) });
  function emit(type, expected, fields = {}) {
    handlers.get(type)?.({ type, ...fields }, ctx);
    assert.equal(readFileSync(process.env.ORCHESTRA_STATUS_FILE, "utf8"), expected, type);
  }
  emit("session_start", "waiting");
  assert.equal(readFileSync(process.env.ORCHESTRA_AGENT_ID_FILE, "utf8"), id);
  emit("before_agent_start", "working");
  emit("agent_start", "working");
  emit("agent_end", "working"); // No premature green while the harness may retry.
  emit("ui_prompt_start", "waiting");
  emit("ui_prompt_start", "waiting");
  emit("ui_prompt_end", "waiting");
  emit("ui_prompt_end", "working");
  emit("session_before_compact", "working");
  emit("session_compact", "working", { willRetry: true });
  emit("agent_settled", "waiting");
  for (const result of ["session_compact", "session_compact_failed"]) {
    emit("session_before_compact", "working");
    emit(result, "waiting", { willRetry: false });
  }
  id = "conversation-b";
  emit("session_start", "waiting");
  assert.equal(readFileSync(process.env.ORCHESTRA_AGENT_ID_FILE, "utf8"), id);
  emit("session_shutdown", "waiting", { reason: "reload" });
  emit("session_shutdown", "ended", { reason: "quit" });
  process.env.ORCHESTRA_STATUS_FILE = join(folder, "missing", "status");
  process.env.ORCHESTRA_AGENT_ID_FILE = join(folder, "missing", "id");
  assert.doesNotThrow(() => handlers.get("session_start")({}, ctx));
  console.log("PASS: Pi activity, queued work, prompts, compaction, conversation IDs and cleanup");
} finally {
  keys.forEach((key, i) => {
    if (saved[i] === undefined) delete process.env[key];
    else process.env[key] = saved[i];
  });
  rmSync(folder, { recursive: true, force: true });
}
