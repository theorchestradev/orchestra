#!/bin/sh
# Signs, notarizes and packages build/bin/Orchestra.app for a release.
#
# Usage: scripts/package-macos.sh VERSION
#
# Signing uses the "Developer ID Application" identity in the keychain, or
# MACOS_SIGN_IDENTITY when set. Notarization uses one of:
#   NOTARY_PROFILE                                     a notarytool keychain profile
#   APPLE_API_KEY_PATH, APPLE_API_KEY_ID, APPLE_API_ISSUER  an App Store Connect API key
#
# Writes dist/Orchestra-VERSION-macos-universal.dmg (first install) and
# dist/Orchestra-VERSION-macos-universal.zip (updates), both stapled.
#
# UNSIGNED=1 skips the Developer ID and notarization and signs ad hoc instead.
# Gatekeeper then blocks the downloaded app until its quarantine attribute is removed.
set -eu

version=${1:?Usage: scripts/package-macos.sh VERSION}
app=build/bin/Orchestra.app
name=Orchestra-$version-macos-universal
dmg=dist/$name.dmg
zip=dist/$name.zip

if [ ! -d "$app" ]; then
	echo "$app is missing. Run make build VERSION=$version PLATFORM=darwin/universal first." >&2
	exit 1
fi
built=$(plutil -extract CFBundleShortVersionString raw "$app/Contents/Info.plist")
if [ "$built" != "$version" ]; then
	echo "$app has version $built, not $version." >&2
	exit 1
fi

make_dmg() {
	rm -rf dist/stage "$dmg" "$zip"
	mkdir -p dist/stage
	ditto "$app" dist/stage/Orchestra.app
	ln -s /Applications dist/stage/Applications
	hdiutil create -volname Orchestra -srcfolder dist/stage -fs HFS+ -format UDZO -ov "$dmg"
	rm -rf dist/stage
}

if [ "${UNSIGNED:-}" = 1 ]; then
	codesign --force --sign - "$app"
	make_dmg
	ditto -c -k --sequesterRsrc --keepParent "$app" "$zip"
	exit 0
fi

identity=${MACOS_SIGN_IDENTITY:-}
if [ -z "$identity" ]; then
	identity=$(security find-identity -v -p codesigning | sed -n 's/.*"\(Developer ID Application: .*\)"/\1/p' | head -1)
fi
if [ -z "$identity" ]; then
	echo "No Developer ID Application identity found. Set MACOS_SIGN_IDENTITY or import the certificate." >&2
	exit 1
fi

if [ -n "${NOTARY_PROFILE:-}" ]; then
	set -- --keychain-profile "$NOTARY_PROFILE"
elif [ -n "${APPLE_API_KEY_PATH:-}" ]; then
	set -- --key "$APPLE_API_KEY_PATH" --key-id "${APPLE_API_KEY_ID:?}" --issuer "${APPLE_API_ISSUER:?}"
else
	echo "Set NOTARY_PROFILE or APPLE_API_KEY_PATH, APPLE_API_KEY_ID and APPLE_API_ISSUER." >&2
	exit 1
fi

# Gatekeeper requires the hardened runtime and a secure timestamp for notarization.
codesign --force --options runtime --timestamp --sign "$identity" "$app"
codesign --verify --strict --verbose=2 "$app"

make_dmg
codesign --timestamp --sign "$identity" "$dmg"

# Notarizing the disk image also notarizes the app inside it, so both can be stapled.
xcrun notarytool submit "$dmg" "$@" --wait
staple() {
	attempt=1
	while :; do
		if xcrun stapler staple "$1"; then return 0; else status=$?; fi
		# Error 68 includes interrupted CloudKit connections after notarization succeeds.
		if [ "$status" -ne 68 ] || [ "$attempt" -ge 3 ]; then return "$status"; fi
		echo "Stapling failed with error 68; retrying in 10 seconds ($attempt/3)." >&2
		sleep 10
		attempt=$((attempt + 1))
	done
}
staple "$dmg"
staple "$app"
spctl --assess --type execute --verbose=2 "$app"
spctl --assess --type open --context context:primary-signature --verbose=2 "$dmg"

ditto -c -k --sequesterRsrc --keepParent "$app" "$zip"
