import { realpathSync, writeFileSync } from "node:fs";

// Injected only into this CLI process, without editing global or project settings.
export const OrchestraSessionPlugin = async ({ directory }) => {
  const output = process.env.ORCHESTRA_AGENT_ID_FILE;
  let active = process.env.ORCHESTRA_RESUME_ID;
  const canonical = (path) => {
    try { return realpathSync(path); } catch { return path; }
  };
  const publish = (id) => {
    if (!output || !/^ses_[a-zA-Z0-9]+$/.test(id ?? "")) return;
    try { writeFileSync(output, id, { mode: 0o600 }); } catch {
      // The terminal may already have closed.
    }
  };
  publish(active);
  return {
    event: async ({ event }) => {
      if (event.type !== "session.created" && event.type !== "session.updated") return;
      const info = event.properties?.info;
      if (!info || info.parentID || canonical(info.directory) !== canonical(directory)) return;
      if (event.type === "session.updated" && active && active !== info.id) return;
      active = info.id;
      publish(active);
    },
  };
};
