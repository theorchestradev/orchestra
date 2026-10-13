import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
const source = readFileSync(new URL("../internal/core/resources/OpenCodeSessionPlugin.js", import.meta.url), "utf8");
const { OrchestraSessionPlugin } = await import(`data:text/javascript;base64,${Buffer.from(source).toString("base64")}`);

const root = mkdtempSync(join(tmpdir(), "orchestra-opencode-"));
const saved = ["ORCHESTRA_AGENT_ID_FILE", "ORCHESTRA_RESUME_ID"].map(key => [key, process.env[key]]);
try {
  delete process.env.ORCHESTRA_RESUME_ID;
  process.env.ORCHESTRA_AGENT_ID_FILE = join(root, "first");
  const first = await OrchestraSessionPlugin({ directory: root });
  process.env.ORCHESTRA_AGENT_ID_FILE = join(root, "second");
  const second = await OrchestraSessionPlugin({ directory: root });
  const event = (type, id, directory = root, parentID) => ({ event: { type, properties: { info: { id, directory, parentID } } } });
  await first.event(event("session.created", "ses_first"));
  await second.event(event("session.created", "ses_second"));
  await first.event(event("session.created", "ses_child", root, "ses_first"));
  await first.event(event("session.created", "ses_otherProject", "/other/project"));
  await first.event(event("session.updated", "ses_anotherRoot"));
  assert.equal(readFileSync(join(root, "first"), "utf8"), "ses_first");
  assert.equal(readFileSync(join(root, "second"), "utf8"), "ses_second");
  await first.event(event("session.created", "ses_new"));
  assert.equal(readFileSync(join(root, "first"), "utf8"), "ses_new");
  process.env.ORCHESTRA_AGENT_ID_FILE = join(root, "resumed");
  process.env.ORCHESTRA_RESUME_ID = "ses_previous";
  const resumed = await OrchestraSessionPlugin({ directory: root });
  await resumed.event(event("session.updated", "ses_unrelated"));
  assert.equal(readFileSync(join(root, "resumed"), "utf8"), "ses_previous");
  console.log("PASS: OpenCode root IDs, resume, new conversation, child/project filtering and concurrent session isolation");
} finally {
  for (const [key, value] of saved) {
    if (value === undefined) delete process.env[key]; else process.env[key] = value;
  }
  rmSync(root, { recursive: true, force: true });
}
