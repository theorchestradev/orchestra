---
name: release
description: Prepare, publish, and verify an Orchestra app release, including changelog notes and macOS/Linux artifacts. Use when a release is requested, not for local builds.
---

# Release Orchestra

- Inspect Git status, fetch origin and tags, and review changes since the latest
  published release. Preserve unrelated work. Use the requested version or infer
  SemVer from the changes. SemVer should be preferred.
- Update `CHANGELOG.md` before tagging: dated, user-facing English notes covering
  the actual changes. Replace the consumed Unreleased section with the version.
- Follow `AGENTS.md` and the existing build and release workflows. Pass the version
  through `make build VERSION=<version>`; do not bump unrelated version fields.
- Commit and integrate the intended changes, push within existing authorization,
  and await successful CI on that exact commit. Then push an annotated
  `v<version>` tag and monitor `.github/workflows/release.yml` through publication.
- Inspect failed logs before retrying. Rerun a transient failure once; if repeated,
  diagnose before another attempt. Reuse bounded packaging retries. Never bypass
  signing, notarization or checksum verification. Report persistent blockers.
- Require specific authorization to replace a pushed tag; prefer a new version.
- Verify release status, latest status for stable releases, and published notes
  against the committed changelog. Download all artifacts and verify SHA256SUMS.
  Confirm macOS signing and notarization succeeded. Preserve existing asset names,
  checksum format and bundle identifier.
- Report the release URL, version, final commit, verification results and any
  remaining local changes or limitations.
