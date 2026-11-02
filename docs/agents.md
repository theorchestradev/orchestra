# Agent support

Orchestra supports 20 agent CLIs, plus plain terminal sessions. Install and sign in
with each provider's CLI before starting it in Orchestra. The app uses the login
shell's `PATH`; it does not install agents or manage their credentials. A missing
CLI produces a launch error in its terminal; install it and start the session again.

Every adapter uses the existing terminal session, repository, worktree, background
session host and Inbox flows. `agentSpec` declares model and account capabilities;
provider-specific preparation stays in its adapter. Model queries use the selected
profile's account, in a neutral directory, without submitting a model prompt.
See the [contract audit](#contract-audit) for the review of all 20 adapters,
regressions covered and the boundary between source review and live verification.

## Additional adapters

| Agent | Executable | Activity and conversation capture | Model choice | Initial GitHub request |
| --- | --- | --- | --- | --- |
| [Cursor CLI](https://cursor.com/docs/cli/reference/parameters) | `cursor-agent` | Hooks; creates an empty chat before attaching the TUI | Native catalog | Yes |
| [GitHub Copilot CLI](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference) | `copilot` | Hooks; dedicated UUID | Model ID and effort | Yes |
| [Aider](https://aider.chat/docs/config/options.html) | `aider` | Completion notifications; separate persistent history per conversation | Native catalog | No |
| [Goose](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/goose-cli-commands.md) | `goose` | Hooks; captures actual session ID | Model ID | Yes |
| [Qwen Code](https://github.com/QwenLM/qwen-code/blob/main/packages/cli/src/config/top-level-options.ts) | `qwen` | Hooks; dedicated UUID | Model ID | Yes |
| [Kiro CLI](https://kiro.dev/docs/reference/cli-commands/) | `kiro-cli` | V3 hooks; captures session ID | Model ID | Yes |
| [Factory Droid](https://docs.factory.com/harness/hooks) | `droid` | Hooks; captures session ID | Model ID and effort | Yes |
| [Kimi Code CLI](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/hooks.html) | `kimi` | Hooks; captures session ID | Configured model ID | No |
| [Amp](https://ampcode.com/docs/plugin-api) | `amp` | Plugin events; captures thread ID | CLI default | No |

Use current CLI versions that support these arguments and hooks. Kiro starts with
`--v3`; Amp needs the current plugin API. Cursor's executable must be `cursor-agent`,
rather than the `cursor` editor launcher. Kimi Code CLI replaces the archived Python
Kimi CLI. Installed versions without the required APIs may fail to start or leave
activity **Unknown**.

Model and effort choices are available in the GitHub **Work on**, **Review** and
**Resolve conflicts** options. Where a CLI has no safe catalog query, enter the exact
model ID configured in that CLI. Leave the field blank to retain its default.
Effort support depends on the selected model. Invalid provider/model combinations
are reported by the CLI. Droid receives choices in a temporary runtime settings
file; Goose receives `GOOSE_MODEL`. Neither changes the user's saved model settings.

Aider, Kimi and Amp are available through **New Session**. Their adapters do not
submit an initial prompt: Aider's `--message` exits after the turn, Kimi's
`--prompt` runs headlessly with automatic permissions, and Amp documents piped
input rather than a TTY-preserving initial prompt argument. These agents are
marked unavailable in GitHub request shortcuts. Type the request in their
terminal, or submit an Inbox request through the existing **Execute Request**
action after starting the session. Saved initial prompts are rejected explicitly.

## Activity and resume

Hooks report working/waiting activity and conversation IDs through the existing
Orchestra CLI. They are observers: they do not approve tools, inject prompts or
forward Inbox responses. Notifications unrelated to permissions or idle status
are ignored. Approval and structured-input dialogs report waiting. Recoverable
Copilot errors and Qwen classifier denials do not imply an idle turn.
Child-agent events cannot replace the root conversation ID or mark
its turn complete. Providers expose different events: Cursor, Goose, Kiro and Amp
may remain **Working** while awaiting a tool approval; Aider reports completion
only and cannot report every turn start or approval.

Per-launch plugins/settings for Cursor, Copilot and Droid live in Orchestra's
runtime directory. The other CLIs discover small managed files in their documented
user plugin/hook locations:

- Qwen: `$QWEN_HOME/extensions/orchestra-session-status/` (default `~/.qwen/`).
- Goose: `~/.agents/plugins/orchestra-session-status/`.
- Kiro: `$KIRO_HOME/hooks/orchestra-session-status.json` (default `~/.kiro/`).
- Amp system plugin: `$XDG_CONFIG_HOME/amp/plugins/orchestra-session-status.ts`
  (default `~/.config/amp/plugins/`).
- Kimi: a marked block of `[[hooks]]` entries in
  `$KIMI_CODE_HOME/config.toml` (default `~/.kimi-code/`). User configuration
  outside the block is preserved. An existing inline `hooks = [...]` array must
  be converted to `[[hooks]]` entries before Orchestra can extend it; a conflicting
  file produces an explicit launch error.

Managed files are installed when launching that agent. Their commands are gated
by `ORCHESTRA_AGENT_KIND` and do nothing in unrelated terminals. Orchestra refuses
to overwrite a same-name unmanaged plugin. Provider settings that disable hooks
remain effective. A silent integration leaves activity **Unknown**; terminal
output is not treated as proof that an agent is idle.

Resume always uses the captured conversation ID or Aider history folder. Terminal
resume hints are a fallback for Kiro, Droid and Amp. If no ID was captured,
Orchestra requires **Start New Conversation** explicitly; it never chooses an
arbitrary latest conversation. Aider history lives beside workspace state under
`AgentSessions/Aider/`; starting a new conversation preserves the previous folder.
The other CLIs retain their own conversation storage locations.

Qwen uses its classifier-backed `auto` approval mode. Goose uses `smart_approve`.
The other new adapters retain their interactive CLI permission defaults. No
adapter enables a full permission bypass. Creating a Cursor chat does not submit
a model prompt.

## Accounts and skills

Claude Code, Codex and Cursor support isolated accounts in **Settings → Profiles**.
Cursor uses both `CURSOR_CONFIG_DIR` and `CURSOR_DATA_DIR`, and its sign-in status
is parsed from the CLI's JSON response. Existing launched Cursor sessions keep
using Default when older profiles gain their new Cursor account folder, so an
upgrade does not silently switch their account. Other new CLIs retain their own
account selection; a workspace profile does not isolate those agents.

The Orchestra skill installer includes the verified keys from `skills@1.7.2`
for Cursor, Copilot, Goose, Qwen, Kiro, Droid, Kimi and Amp. Aider has no confirmed
key in that catalog, so it is excluded from skill installation. Cursor's skills
installer ignores its config-folder variable; Orchestra copies the installed
public skill stub into each Cursor profile instead. Credentials are never copied.

## Contract audit

Reviewed on 2026-11-02. Scope: every registered agent in `agentOrder` (20),
including the previous adapters. Plain Terminal is covered by the existing
session tests and is not an agent adapter.

This is a source and contract review, with isolated regression tests. It is not
an end-to-end certification of authenticated provider sessions. No provider
prompts, credentials, sign-in flows or personal settings were used for verification.
CLIs evolve independently; the references below identify the expected contracts,
not a guarantee that every older installed version supports them.

### Contracts checked

All rows were checked for interactive launch, initial prompt handling, explicit
resume identity, available model/effort transport, observer payloads and account
isolation. An unavailable capability stays unavailable; a missing provider API
is not replaced with headless execution or automatic terminal input.

| Agent | Launch, resume and choices | Observation and limits | Primary contract |
| --- | --- | --- | --- |
| Codex | TUI prompt after `--`; `resume ID`; `--approve-for-me`; `-m` and TOML effort override; `debug models`; `CODEX_HOME` accounts | Command hooks use the shared root reporter. Hook trust remains required; no trust bypass is added. Exit banner is an ID fallback. | [CLI](https://developers.openai.com/codex/cli/reference), [source](https://github.com/openai/codex/blob/main/codex-rs/cli/src/main.rs) |
| Claude Code | TUI prompt after `--`; fresh `--session-id` or explicit `--resume`; `--settings` overlay; model/effort flags; `CLAUDE_CONFIG_DIR` accounts | Root-filtered hooks, including permission requests and terminal turn failures. Model aliases are a fixed catalog; available levels still depend on the model. | [CLI](https://code.claude.com/docs/en/cli-reference), [hooks](https://code.claude.com/docs/en/hooks) |
| Pi | `--session-id` opens or creates an exact conversation; `--extension`; model/thinking flags and native catalog | Extension lifecycle, retries, compaction and explicit Inbox reply forwarding. New Orchestra conversations get fresh IDs. | [CLI](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/cli.md), [extensions](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/docs/extensions.md) |
| Mistral Vibe | Positional interactive prompt; explicit `--resume`; process-only `VIBE_SESSION_LOGGING` JSON selects a separate log folder | Captures IDs from newly written metadata. Activity integration and account profiles are not implemented. Logging schemas vary by CLI generation. | [usage and sessions](https://github.com/mistralai/mistral-vibe/blob/main/README.md), installed distribution's `vibe/cli/entrypoint.py` and `vibe/core/config.py` |
| OpenCode | TUI `--prompt`, `--session`, `--model`; native `models`; additive `OPENCODE_CONFIG_CONTENT` plugin | Root session events; permission/question tracking. A new root clears previous approvals and publishes its status immediately. | [CLI](https://opencode.ai/docs/cli/), [plugins](https://opencode.ai/docs/plugins/) |
| Antigravity CLI | `agy --mode accept-edits`; `--prompt-interactive`; explicit `--conversation`; native model catalog | Captures the printed resume hint on exit. No activity hooks or account profiles are claimed. The conservative approval mode is retained. | [CLI](https://antigravity.google/docs/cli/reference/) |
| Grok Build | Positional TUI prompt after `--`; fresh UUID for `--session-id`; explicit `--resume`; permission auto mode; native model catalog | Initial prompts are now passed instead of discarded. Activity hooks and account profiles are not implemented. | [getting started](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/docs/user-guide/01-getting-started.md), [argument parser](https://github.com/xai-org/grok-build/blob/main/crates/codegen/xai-grok-pager/src/app/cli.rs) |
| Cline | `--tui` with positional prompt after `--`; `--id` resumes; `--auto-approve false` retains approvals | Existing managed hook scripts preserve user-owned files. Nested `sessionContext.rootSessionId` goes through the shared reporter. The config-folder installation remains for compatibility with versions that ignore `--hooks-dir`. | [CLI](https://docs.cline.bot/cli/cli-reference), [source](https://github.com/cline/cline) |
| omp | `--approval-mode write`; explicit `--resume`; local extension; model/thinking flags; JSON model catalog | Main-agent lifecycle, approval counts, retries and compaction. Child agent contexts are ignored. | [argument parser](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/cli/args.ts), [extension types](https://github.com/can1357/oh-my-pi/blob/main/packages/coding-agent/src/extensibility/extensions/types.ts) |
| Kilo | OpenCode-derived TUI `--prompt`, `--session`, `--model`; native catalog; `KILO_CONFIG_CONTENT` plugin | Uses the same tested observer as OpenCode. CLI defaults retain permissions. | [CLI and configuration](https://kilo.ai/docs/code-with-ai/platforms/cli), [source](https://github.com/Kilo-Org/kilocode) |
| Muse Code | Interactive prompt after `--`; `resume ID`; `:auto-review` permission profile; inline hook config; model/effort flags | Hook commands explicitly restore Orchestra paths because the CLI filters their environment. Root filtering and `StopFailure` handling are shared. Model catalog/effort choices are a conservative fixed subset. | [configuration](https://dev.meta.ai/docs/muse-code/configuration), [extending](https://dev.meta.ai/docs/muse-code/extending), [interactive sessions](https://dev.meta.ai/docs/muse-code/interactive) |
| Cursor CLI | Empty `create-chat` followed by exact `--resume`; TUI prompt; process plugin; native catalog; isolated config and data folders | Observer events avoid tool permission decisions. A failed chat creation cannot fall back to latest. JSON sign-in status is parsed. Skill copying also respects a custom Default config folder. | [parameters](https://cursor.com/docs/cli/reference/parameters), [hooks](https://cursor.com/docs/hooks), [plugins](https://cursor.com/docs/reference/plugins) |
| GitHub Copilot CLI | Exact UUID with `--session-id` creates or resumes; `--interactive` prompt; local plugin; model/effort flags | Recoverable errors do not mark the session idle. Notifications are filtered by type. Account profiles remain unsupported. | [CLI](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference), [hooks](https://docs.github.com/en/copilot/reference/hooks-reference) |
| Aider | Separate persistent chat/input histories; `--restore-chat-history`; model flag and `--list-models` catalog | Notifications report completion only. `--message` exits, so initial GitHub prompts remain unsupported. | [options](https://aider.chat/docs/config/options.html) |
| Goose | `session` or `run --interactive --text`; exact `--session-id` on resume; `GOOSE_MODEL`; `smart_approve` | Managed system plugin captures the actual ID. No reliable permission-wait event is claimed. | [commands](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/goose-cli-commands.md), [hooks](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/hooks.md) |
| Qwen Code | UUID `--session-id`; explicit `--resume`; `--prompt-interactive`; `--model`; classifier auto mode | Extension hooks: classifier denials mean continued work, permission dialogs mean waiting, and `StopFailure` means an ended turn. | [arguments](https://github.com/QwenLM/qwen-code/blob/main/packages/cli/src/config/top-level-options.ts), [hooks](https://github.com/QwenLM/qwen-code/blob/main/docs/users/features/hooks.md), [extension loading](https://github.com/QwenLM/qwen-code/blob/main/packages/core/src/extension/extensionManager.ts) |
| Kiro CLI | `chat --v3`; positional prompt; `--resume-id`; model flag | Standalone v1 hook schema in the user hooks directory, not the old embedded agent schema. Permission-wait reporting remains partial. Requires the V3 contract. | [commands](https://kiro.dev/docs/reference/cli-commands/), [V3 migration](https://kiro.dev/docs/cli/v3/hooks-migration/) |
| Factory Droid | `--settings` process-only file; explicit `--resume`; positional prompt; model/reasoning settings | Grouped hooks and notification types follow the harness schema. Runtime overlay support was checked against the public installed 0.135.0 help captured during implementation. Coexistence with every user/project hook configuration has not been exercised against a running provider CLI. | [CLI](https://github.com/Factory-AI/factory/blob/main/docs/reference/cli-reference.mdx), [hooks](https://docs.factory.com/harness/hooks) |
| Kimi Code CLI | `--session ID`; `--model` configured alias; no initial prompt because `--prompt` is headless | Marked `[[hooks]]` block preserves user configuration. Covers permission result, tool failure, turn failure and interruption. Conflicting inline hook arrays produce an explicit error. | [command](https://www.kimi.com/code/docs/en/kimi-code-cli/reference/kimi-command.html), [hooks](https://www.kimi.com/code/docs/en/kimi-code-cli/customization/hooks.html) |
| Amp | Interactive TUI; exact `threads continue ID`; no headless prompt workaround or model catalog | System plugin follows `activeThread` and preserves per-thread activity. Events from background threads cannot replace the selected ID; explicit thread switches update resume identity. Requires the current plugin API. | [CLI](https://ampcode.com/docs/cli), [plugin API](https://ampcode.com/docs/plugin-api) |

### Regression coverage

- All 20 command builders run against isolated fake CLIs, for fresh and resumed
  sessions, with prompts starting like flags, quotes, newlines and shell syntax.
  Model choices are checked at their flag, environment or settings boundary.
- Background quit/reopen preserves plugin directories and runtime settings while
  processes are live, but removes stale activity files. Integration assets can be
  reclaimed once there are no live host sessions.
- Amp tests switch the selected thread, ignore background starts/completions and
  verify the selected conversation ID and activity together.
- Copilot tests distinguish recoverable errors from terminal errors, and ignore
  unrelated notifications while recognizing approval and elicitation dialogs.
- Claude, Pi and Grok tests start multiple new conversations in one Orchestra
  session and verify distinct IDs are passed to the actual shell command.
- OpenCode/Kilo's smoke test switches root conversations while an old permission
  prompt is pending and checks that it cannot keep the new root waiting.
- Launch preparation is exported after shell startup, so rc files cannot discard
  process-only model and integration overlays.
- Existing Pi, omp, profile isolation, sign-in parsing, skill copying, history,
  quoting, state persistence and session-host checks remain in the verification set.

### Remaining verification boundary

Authenticated live runs were not part of this audit. In particular, provider
availability, model entitlements, older CLI releases, terminal UI behavior and
provider-side hook loading still need an isolated native CLI smoke pass before
claiming end-to-end parity with Claude. The capability limits described above
remain; a passing fake CLI does not establish provider parity.

## Brand assets

Cursor, GitHub Copilot, Goose, Qwen, Kiro, Kimi and Amp marks come from
[LobeHub Icons](https://github.com/lobehub/lobe-icons) (MIT). Aider's mark comes
from its [official assets](https://github.com/Aider-AI/aider/tree/main/aider/website/assets/icons);
Factory's comes from its [official favicon](https://factory.com/favicon.svg).
Brand marks retain their original colors, with light/dark variants for
monochrome marks. Trademarks belong to their respective owners.
