import { writeFileSync } from "node:fs";

// Loaded only for this CLI process; never changes the user's omp settings.
export default function (pi) {
  let working = false;
  let compacting = false;
  let approvals = 0;

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
    write(process.env.ORCHESTRA_STATUS_FILE, (working || compacting) && approvals === 0 ? "working" : "waiting");
  }

  // omp binds extensions to subagents and the advisor too; only the main session reports.
  function on(events, update) {
    for (const event of events) {
      pi.on(event, (_event, ctx) => {
        if (ctx.agent && ctx.agent.kind !== "main") return;
        update();
        publish(ctx);
      });
    }
  }

  on(["session_start", "session_switch", "session_branch"], () => { working = compacting = false; approvals = 0; });
  on(["before_agent_start", "agent_start", "auto_retry_start"], () => { working = true; });
  on(["auto_compaction_start"], () => { compacting = true; });
  on(["auto_compaction_end"], () => { compacting = false; });
  on(["agent_end"], () => { working = false; approvals = 0; });
  on(["tool_approval_requested"], () => { approvals++; });
  on(["tool_approval_resolved"], () => { approvals = Math.max(0, approvals - 1); });
}
