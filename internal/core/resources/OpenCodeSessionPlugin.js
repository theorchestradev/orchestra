import { realpathSync, writeFileSync } from "node:fs";

// Injected only into this CLI process, without editing global or project settings.
// OpenCode and Kilo share it: it reports the root conversation ID and the session status.
export const OrchestraSessionPlugin = async ({ directory }) => {
  const output = process.env.ORCHESTRA_AGENT_ID_FILE;
  let active = process.env.ORCHESTRA_RESUME_ID;
  let busy = false;
  // Permission and question prompts wait for the user, including those from subagents.
  const pending = new Set();
  const canonical = (path) => {
    try { return realpathSync(path); } catch { return path; }
  };
  const write = (path, value) => {
    if (!path) return;
    try { writeFileSync(path, value, { mode: 0o600 }); } catch {
      // The terminal may already have closed.
    }
  };
  const publish = (id) => {
    if (/^ses_[a-zA-Z0-9]+$/.test(id ?? "")) write(output, id);
  };
  const status = () => write(process.env.ORCHESTRA_STATUS_FILE, busy && pending.size === 0 ? "working" : "waiting");
  publish(active);
  status();
  return {
    event: async ({ event }) => {
      const properties = event.properties ?? {};
      switch (event.type) {
        case "session.created":
        case "session.updated": {
          const info = properties.info;
          if (!info || info.parentID || canonical(info.directory) !== canonical(directory)) return;
          if (event.type === "session.updated" && active && active !== info.id) return;
          if (active !== info.id) busy = false;
          active = info.id;
          publish(active);
          return;
        }
        case "session.status":
          if (properties.sessionID !== active) return;
          busy = properties.status?.type === "busy" || properties.status?.type === "retry";
          break;
        case "permission.asked":
        case "question.asked":
          pending.add(properties.id);
          break;
        case "permission.replied":
        case "question.replied":
        case "question.rejected":
          pending.delete(properties.requestID);
          break;
        default:
          return;
      }
      status();
    },
  };
};
