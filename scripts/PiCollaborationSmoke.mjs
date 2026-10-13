import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";

const source = readFileSync(new URL("../internal/core/resources/PiStatusExtension.js", import.meta.url), "utf8");
const { default: register } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);
const keys = ["ORCHESTRA_SOCKET", "ORCHESTRA_SESSION_ID", "ORCHESTRA_STATUS_FILE", "ORCHESTRA_AGENT_ID_FILE"];
const saved = keys.map(key => process.env[key]);
try {
  process.env.ORCHESTRA_SOCKET = "/tmp/orchestra-pi-smoke/broker.sock";
  process.env.ORCHESTRA_SESSION_ID = "recipient";
  delete process.env.ORCHESTRA_STATUS_FILE;
  delete process.env.ORCHESTRA_AGENT_ID_FILE;
  const id = "fef66e62-6180-4150-b8e8-8bfebb0ad438";
  const handlers = new Map(), replies = [], notices = [];
  let run, disconnected = false;
  const ctx = { sessionManager: { getSessionId: () => "conversation" }, ui: { notify: text => notices.push(text) } };
  register({
    on: (event, handler) => handlers.set(event, handler),
    exec: async (command, args, options) => {
      assert.equal(command, join(dirname(process.env.ORCHESTRA_SOCKET), "orchestra"));
      assert.equal(options.timeout, 10000);
      assert.deepEqual(args.slice(0, 1), ["run"]);
      assert.equal(args[2], id);
      if (disconnected) return { code: 1, stderr: "Broker unavailable", stdout: "" };
      if (args[1] === "reply") {
        assert.equal(run.status, "running");
        assert.equal(args[3], "--text");
        replies.push(args[4]);
        run = { ...run, status: "completed", response: args[4] };
      } else assert.equal(args[1], "read");
      return { code: 0, stdout: JSON.stringify(run), stderr: "" };
    },
  });
  const emit = async (type, fields = {}) => await handlers.get(type)?.({ type, ...fields }, ctx);
  async function start(status = "running", recipient = "recipient") {
    await emit("session_start");
    run = { id, status, toSessionID: recipient };
    await emit("before_agent_start", { prompt: `Orchestra request ${id} from Claude Code 1:\n\nQual é a capital da França?` });
    await emit("agent_start");
  }
  async function end(text, stopReason = "stop") {
    await emit("agent_end", { messages: [
      { role: "toolResult", content: [{ type: "text", text: "private tool output" }] },
      { role: "assistant", stopReason, content: [{ type: "thinking", thinking: "private reasoning" }, { type: "text", text }] },
    ] });
  }
  await start();
  await end("A capital da França é **Paris**.");
  assert.equal(replies.length, 0, "wait for final settlement");
  await emit("agent_settled");
  assert.deepEqual(replies, ["A capital da França é **Paris**."]);
  await emit("agent_settled");
  assert.equal(replies.length, 1, "do not forward twice");

  for (const reason of ["error", "aborted", "length", "toolUse"]) {
    await start(); await end("incomplete", reason); await emit("agent_settled");
    assert.equal(replies.length, 1, reason);
  }
  await start(); await end("retry failure", "error"); await end("Paris after retry");
  await emit("agent_settled");
  assert.equal(replies.at(-1), "Paris after retry");

  for (const status of ["cancelled", "completed"]) {
    await start(); await end("do not replace existing result"); run.status = status;
    await emit("agent_settled"); assert.equal(replies.length, 2, status);
  }
  for (const [status, recipient] of [["queued", "recipient"], ["running", "another session"]]) {
    await start(status, recipient); await end("not an executed request for this session");
    await emit("agent_settled"); assert.equal(replies.length, 2);
  }
  await start();
  await emit("before_agent_start", { prompt: "An unrelated user question" });
  await end("Unrelated answer"); await emit("agent_settled");
  assert.equal(replies.length, 2, "normal turns must not complete a request");

  await start(); await end("Original request answer");
  await emit("before_agent_start", { prompt: "Queued ordinary question" });
  await end("Queued ordinary answer"); await emit("agent_settled");
  assert.equal(replies.at(-1), "Original request answer", "do not confuse queued responses");

  await start();
  await emit("agent_end", { messages: [
    { role: "user", content: [{ type: "text", text: `Orchestra request ${id} from Claude Code 1:\n\nQuestion` }] },
    { role: "assistant", stopReason: "stop", content: [{ type: "text", text: "Answer before queued input" }] },
    { role: "user", content: "Unrelated input drained in the same agent loop" },
    { role: "assistant", stopReason: "stop", content: [{ type: "text", text: "Unrelated queued response" }] },
  ] });
  await emit("agent_settled");
  assert.equal(replies.at(-1), "Answer before queued input");

  await start();
  await emit("agent_end", { messages: [
    { role: "user", content: "Unrelated steering input before an answer" },
    { role: "assistant", stopReason: "stop", content: [{ type: "text", text: "Unrelated response" }] },
  ] });
  await emit("agent_settled"); assert.equal(replies.length, 4);

  await start(); await end("Discarded by session switch"); await emit("session_start");
  await emit("agent_settled"); assert.equal(replies.length, 4);
  await start(); await end("Cannot publish"); disconnected = true;
  await emit("agent_settled"); assert.equal(replies.length, 4);
  assert.match(notices.at(-1), /could not forward.*Broker unavailable/);
  console.log("PASS: Pi final replies, retries, cancellation, explicit replies, session ownership and unrelated turns");
} finally {
  keys.forEach((key, i) => saved[i] === undefined ? delete process.env[key] : process.env[key] = saved[i]);
}
