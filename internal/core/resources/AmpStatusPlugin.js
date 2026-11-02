// Orchestra session status plugin.
import { writeFileSync } from "node:fs";

// System plugins are loaded everywhere; stay inert outside an Orchestra Amp session.
export default function (amp) {
  if (process.env.ORCHESTRA_AGENT_KIND !== "amp") return;
  let root = process.env.ORCHESTRA_ROOT_AGENT_ID || undefined;
  const states = new Map();
  function publish(state, id) {
    if (!id || id !== root) return;
    try {
      writeFileSync(process.env.ORCHESTRA_AGENT_ID_FILE, id, { mode: 0o600 });
      writeFileSync(process.env.ORCHESTRA_STATUS_FILE, state, { mode: 0o600 });
    } catch {
      // The session may have been closed while a hook was pending.
    }
  }
  // The active thread is authoritative. session.start also fires for background
  // threads, so it cannot by itself identify a user-selected conversation.
  function select(thread) {
    root = thread?.id;
    publish(states.get(root) || "waiting", root);
  }
  const subscription = amp.activeThread.subscribe(select);
  amp.onDispose(() => subscription.unsubscribe());
  if (amp.activeThread.current) select(amp.activeThread.current);
  amp.on("session.start", (event) => {
    if (amp.activeThread.current?.id === event.thread.id) select(event.thread);
  });
  function update(state, event) {
    states.set(event.thread.id, state);
    publish(state, event.thread.id);
  }
  amp.on("agent.start", (event) => update("working", event));
  amp.on("agent.end", (event) => update("waiting", event));
}
