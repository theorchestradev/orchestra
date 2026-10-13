import { writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

// Loaded only for this CLI process; never changes the user's Pi settings.
export default function (pi) {
  let working = false;
  let compacting = false;
  let prompts = 0;
  let request = null;
  let response = null;

  async function broker(action, id, text) {
    const result = await pi.exec(join(dirname(process.env.ORCHESTRA_SOCKET), "orchestra"),
      ["run", action, id, ...(text === undefined ? [] : ["--text", text])], { timeout: 10000 });
    if (result.code !== 0) throw new Error(result.stderr || "Could not connect to Orchestra.");
    return JSON.parse(result.stdout);
  }

  function replyError(ctx, error) {
    ctx.ui?.notify(`Orchestra could not forward the reply: ${error.message}`, "error");
  }

  function write(path, value) {
    if (!path) return;
    try {
      writeFileSync(path, value, { mode: 0o600 });
    } catch {
      // Orchestra may already have removed the files while closing the terminal.
    }
  }

  function publish(ctx) {
    write(process.env.ORCHESTRA_AGENT_ID_FILE, ctx.sessionManager.getSessionId());
    write(process.env.ORCHESTRA_STATUS_FILE,
      prompts > 0 || (!working && !compacting) ? "waiting" : "working");
  }

  pi.on("session_start", (_event, ctx) => {
    working = compacting = false;
    prompts = 0;
    request = response = null;
    publish(ctx);
  });
  pi.on("before_agent_start", async (event, ctx) => {
    working = true;
    publish(ctx);
    request = null;
    const id = event.prompt?.match(/^Orchestra request ([0-9a-f-]{36}) from /i)?.[1];
    if (!id || !process.env.ORCHESTRA_SOCKET) return;
    try {
      const run = await broker("read", id);
      if (run.status === "running" && run.toSessionID === process.env.ORCHESTRA_SESSION_ID) request = run.id;
    } catch (error) { replyError(ctx, error); }
  });
  pi.on("agent_start", (_event, ctx) => { working = true; publish(ctx); });
  pi.on("agent_end", event => {
    if (!request) return;
    let replyingTo = request;
    for (const message of event.messages || []) {
      if (message.role === "user") {
        const input = typeof message.content === "string" ? message.content
          : message.content.filter(part => part.type === "text").map(part => part.text).join("\n");
        replyingTo = input.startsWith(`Orchestra request ${request} from `) ? request : null;
      }
      if (replyingTo && message.role === "assistant") {
        const text = message.stopReason === "stop"
          ? message.content.filter(part => part.type === "text").map(part => part.text).join("\n").trim() : "";
        // Never forward reasoning, tool output, interrupted turns or another user prompt.
        response = text ? { id: replyingTo, text } : null;
      }
    }
    // Pi can drain queued user messages inside one loop without before_agent_start.
    request = replyingTo;
  });
  // agent_end can precede automatic retries or queued work. Wait for the final idle event.
  pi.on("agent_settled", async (_event, ctx) => {
    working = compacting = false;
    publish(ctx);
    const reply = response;
    request = response = null;
    if (!reply) return;
    try {
      const run = await broker("read", reply.id);
      // An explicit CLI reply or cancellation wins over automatic forwarding.
      if (run.status === "running" && run.toSessionID === process.env.ORCHESTRA_SESSION_ID) {
        await broker("reply", reply.id, reply.text);
      }
    } catch (error) { replyError(ctx, error); }
  });
  pi.on("session_before_compact", (_event, ctx) => { compacting = true; publish(ctx); });
  for (const event of ["session_compact", "session_compact_failed"]) {
    pi.on(event, (event, ctx) => {
      compacting = event.willRetry;
      publish(ctx);
    });
  }
  pi.on("ui_prompt_start", (_event, ctx) => { prompts++; publish(ctx); });
  pi.on("ui_prompt_end", (_event, ctx) => { prompts = Math.max(0, prompts - 1); publish(ctx); });
  pi.on("session_shutdown", (event) => {
    if (event.reason === "quit") write(process.env.ORCHESTRA_STATUS_FILE, "ended");
  });
}
