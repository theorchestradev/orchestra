# Releasing Orchestra

A SemVer tag is the only source of the version. Pushing a `v*` tag runs
`.github/workflows/release.yml`, which builds both platforms and publishes a GitHub
Release with generated notes:

| Asset | Use |
| --- | --- |
| `Orchestra-VERSION-macos-universal.dmg` | First install on macOS (arm64 and x86_64) |
| `Orchestra-macos-universal.dmg` | The same disk image without the version, so `releases/latest/download/Orchestra-macos-universal.dmg` always serves the latest release |
| `Orchestra-VERSION-macos-universal.zip` | The same signed app, for updates |
| `Orchestra-VERSION-linux-x86_64.tar.gz` | The Linux binary, `LICENSE` and the icon |
| `SHA256SUMS` | Checksums of the assets above |

## Cutting a release

```sh
git switch main && git pull
git tag v0.2.0
git push origin v0.2.0
```

The in-app updater downloads these assets by name, so keep the names and the
`SHA256SUMS` format stable.

A tag with a pre-release suffix such as `v0.2.0-beta.1` is published as a
pre-release. The workflow rejects tags that are not SemVer.

`make build VERSION=0.2.0` injects the version with `-ldflags -X main.version=…` and,
on macOS, writes it into the bundle's `Info.plist`. Builds without `VERSION` report
`dev` and keep the worktree and branch in the About panel. `orchestra --version`
prints the version, from the app executable and from the CLI in Orchestra sessions.

## Optional usage analytics

Both release build jobs pass `RELEASE_TELEMETRY=1` and read the
`POSTHOG_PROJECT_TOKEN` Actions secret and `POSTHOG_HOST` Actions variable.
The host must match the project's ingestion region (`https://us.i.posthog.com`
for Orchestra's existing project). A missing token disables analytics.
Ordinary `make build` and `wails dev` builds do not embed the token, even when
the PostHog environment variables are set. Users must still opt in before any
event is recorded or sent.

Before publishing, configure the project's IP discard and city/country privacy
transformation as described in [Privacy & usage data](docs/privacy.md).

## macOS signing and notarization

The bundle identifier is `dev.theorchestra.Orchestra` (`build/darwin/Info.plist`).
Keep it stable: updates, preferences and privacy permissions are tied to it.

The app is signed with a Developer ID Application certificate and the hardened
runtime, then the disk image is notarized and both the image and the app are stapled.
`scripts/package-macos.sh` does this in CI and can run locally after a universal
build:

```sh
make build VERSION=0.2.0 PLATFORM=darwin/universal
xcrun notarytool store-credentials orchestra-notary   # once
NOTARY_PROFILE=orchestra-notary scripts/package-macos.sh 0.2.0
```

The workflow needs these repository secrets:

| Secret | Value |
| --- | --- |
| `MACOS_CERTIFICATE` | The Developer ID Application certificate and private key, exported as `.p12` and base64-encoded |
| `MACOS_CERTIFICATE_PASSWORD` | The `.p12` password |
| `APPLE_API_KEY` | An App Store Connect API key (`AuthKey_….p8`), base64-encoded |
| `APPLE_API_KEY_ID` | The key ID |
| `APPLE_API_ISSUER` | The issuer ID |

Until `MACOS_CERTIFICATE` is set, the workflow signs the app ad hoc, skips
notarization, publishes the release as a pre-release and adds a note: Gatekeeper blocks
the downloaded app until `xattr -dr com.apple.quarantine /Applications/Orchestra.app`
removes its quarantine attribute. `UNSIGNED=1 scripts/package-macos.sh VERSION` does
the same locally.

Create the certificate in the Apple Developer account under Certificates → Developer
ID Application (only the Account Holder can), and the API key in App Store Connect →
Users and Access → Integrations with the Developer role. Set the secrets with
`base64 -i file | gh secret set NAME`.

## Linux

The Linux build uses `-tags webkit2_41` on Ubuntu 24.04, like `linux.yml`, and needs
`libgtk-3` and `libwebkit2gtk-4.1` at runtime.
