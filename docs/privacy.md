# Privacy & usage data

Orchestra sends no usage data until you choose **Share Anonymous Usage** in the
first-launch banner or turn on **Settings → Privacy → Share anonymous usage data**.
**No Thanks** is remembered, and the banner never returns after either choice.

Sharing includes feature usage and approximate city and country. PostHog derives
location from the network connection and discards the IP before storing the event.
The city may be inaccurate or reflect a VPN. The project pipeline retains only
`$geoip_city_name`, `$geoip_country_name` and `$geoip_country_code`: no coordinates,
postal code, region or timezone.

A random installation UUID is created only when you opt in. It is never derived
from agent accounts, email, GitHub, hostname or paths. Turning sharing off cancels
pending requests, discards the memory queue and deletes the UUID. Opting in again
creates a new one. Already delivered events remain in PostHog.

Never collected: prompts, code, terminal input/output, Scratchpad text, inbox
bodies, file names, paths, project/session/repository/branch names, GitHub titles
or numbers, error messages, credentials or anything typed by the user. There is
no autocapture, session replay, person identification or analytics SDK.

## Event inventory

<!-- telemetry-events:start -->
| Event | Properties and allowed values |
| --- | --- |
| `app_opened` | `version`: release version; `os`: `darwin`, `linux`; `arch`: `amd64`, `arm64`, `other`; `theme_kind`: `built-in`, `custom`; `arc_mode`: boolean |
| `project_added` | `has_github_repository`: boolean |
| `session_started` | `agent`: `claude`, `codex`, `pi`, `vibe`, `opencode`, `antigravity`, `grok`, `cline`, `omp`, `terminal`; `worktree`: boolean; `from_github_item`: boolean |
| `session_ended` | `agent`: `claude`, `codex`, `pi`, `vibe`, `opencode`, `antigravity`, `grok`, `cline`, `omp`, `terminal`; `exit_kind`: `normal`, `error`, `killed`; `duration_bucket`: `under_1m`, `1-5m`, `5-30m`, `30m+` |
| `feature_used` | `feature`: `scratchpad_read`, `scratchpad_cli`, `inbox_execute`, `inbox_auto_delivery`, `diff_view`, `command_bar`, `emoji_picker` |
| `github_action` | `action`: `work_on`, `review`, `edit_metadata` |
| `update_installed` | `from_version`: release version; `to_version`: release version |
| `error_shown` | `code`: `operation_failed`, `session_start_failed`, `session_exit_error` |
<!-- telemetry-events:end -->

Every event also has its event name, timestamp and random installation UUID
(`distinct_id`). Go adds fixed transport properties `$process_person_profile: false`
(no person profiles), `$geoip_disable: false` (approximate location enrichment) and
`$lib: orchestra` (scopes the privacy transformation). The PostHog project token
authenticates each batch; it does not identify a user. PostHog adds the three
city/country properties listed above during ingestion.

`update_installed` is sent when quitting replaces the app with an update the user chose
to install.
Durations are bucketed; no exact duration or exit message is sent.

## Environment controls

`DO_NOT_TRACK=1`, `ORCHESTRA_TELEMETRY_DISABLED=1`, CI environment variables and
builds without release-injected configuration disable analytics regardless of
the saved preference. Settings explains the reason. `make build`, `wails dev`
and contributor builds contain no PostHog token.

`ORCHESTRA_TELEMETRY_DEBUG=1` writes validated, consented events to the local
Orchestra log instead of sending them. It does not bypass consent or environment
controls. Debug output contains the outgoing event; location is added by PostHog,
so local debug mode has no location lookup. Normal logs remain local and analytics
never reads them.

Events stay in memory (maximum 256), flush every 60 seconds or at shutdown, retry
at most three times and are then dropped. Each event name is limited to 20 events
per minute. Nothing is written to an analytics file, and no upload blocks the UI.

## Official release configuration

The release workflow must inject these Go linker variables:

- `main.version`: the release tag without its `v` prefix, e.g. `1.0.0`.
- `main.buildTelemetryKey`: the project token from the `POSTHOG_PROJECT_TOKEN` Actions secret.
- `main.buildTelemetryHost`: the `POSTHOG_HOST` Actions variable, configured as
  `https://us.i.posthog.com` for the existing Orchestra
  project (US). EU projects must use `https://eu.i.posthog.com` instead.

The project token secret and host variable are configured in `theorchestradev/orchestra`.
The macOS and Linux jobs in `.github/workflows/release.yml` pass
`RELEASE_TELEMETRY=1` to `make build`, which adds the token and host to the existing
build flags. Ordinary builds ignore the PostHog environment variables, even when
`VERSION` is set. A missing project token disables analytics in the release.
Do not put the token in source, frontend assets or local build defaults.
A runtime environment variable cannot supply a token.

Before shipping a configured release, the target project must have **Discard
client IP data** enabled, GeoIP enabled, and the transformation in
[posthog-location.hog](posthog-location.hog) running immediately after GeoIP. Its
filter is `$lib = orchestra`, so other project traffic is unchanged. This prevents
storing extra location details. Those settings were applied to the existing
Orchestra project during implementation; there is no live event upload in QA.

The website reads `internal/telemetry/events.json` through its app sync script to
build `/docs/privacy`. Run `node scripts/TelemetryDocs.mjs --write` after changing
the inventory; its check is included in the frontend test suite.
