// The release the status bar offers: the newest one found, unless the user skipped it and
// has not already chosen to install it.
export function pendingUpdate(status, preferences = {}) {
  const release = status?.available
  return release && (release.version === status.staged || release.version !== preferences.skippedUpdate)
    ? release
    : null
}
