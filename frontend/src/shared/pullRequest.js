// Check groups in the order they need attention. Statuses come from SessionPullRequest.
export const checkGroups = [
  { status: 'failure', label: 'failing', icon: 'xmark.circle' },
  { status: 'pending', label: 'in progress', icon: 'circle.dot' },
  { status: 'cancelled', label: 'cancelled', icon: 'circle.slash' },
  { status: 'success', label: 'successful', icon: 'checkmark.circle' },
  { status: 'skipped', label: 'skipped', icon: 'circle.slash' },
]

export const stateLabels = { open: 'Open', draft: 'Draft', merged: 'Merged', closed: 'Closed' }
export const stateIcons = {
  open: 'arrow.triangle.pull',
  draft: 'circle.dashed',
  merged: 'arrow.triangle.merge',
  closed: 'xmark.circle',
}

const checkWord = (count) => (count === 1 ? 'check' : 'checks')

// Non-empty groups, each keeping GitHub's order of its checks.
export function groupChecks(checks = []) {
  return checkGroups
    .map((group) => {
      const items = checks.filter((check) => check.status === group.status)
      return { ...group, checks: items, title: `${items.length} ${group.label} ${checkWord(items.length)}` }
    })
    .filter((group) => group.checks.length)
}

// The headline, detail line and worst outcome shown above the checks.
export function checksSummary(checks = []) {
  const groups = groupChecks(checks)
  const has = (status) => groups.some((group) => group.status === status)
  const outcome = !checks.length
    ? 'none'
    : has('failure')
      ? 'failure'
      : has('pending')
        ? 'pending'
        : has('cancelled')
          ? 'cancelled'
          : 'success'
  const headline = {
    none: 'No checks',
    failure: 'Some checks were not successful',
    pending: 'Some checks haven’t completed yet',
    cancelled: 'Some checks were cancelled',
    success: 'All checks have passed',
  }[outcome]
  const detail = checks.length
    ? groups.map((group) => `${group.checks.length} ${group.label}`).join(', ') + ' ' + checkWord(checks.length)
    : 'This pull request has no checks on its latest commit.'
  return { outcome, headline, detail, groups }
}

// Ring segments as fractions of the circle, worst first, clockwise from the top.
export function ringSegments(checks = []) {
  let start = 0
  return groupChecks(checks).map((group) => {
    const size = group.checks.length / checks.length
    const segment = { status: group.status, start, size }
    start += size
    return segment
  })
}

// Polls quickly while checks run and slowly once they settle.
export function refreshDelay(status) {
  return status?.checks?.some((check) => check.status === 'pending') ? 20000 : 120000
}

const reviewLabels = { APPROVED: 'Approved', CHANGES_REQUESTED: 'Requested changes', COMMENTED: 'Reviewed' }

// Reviews and comments, oldest first. Pending reviews are drafts only their author sees, and
// a plain review with no text only repeats its line comments.
export function pullRequestActivity(status) {
  const items = []
  for (const review of status?.reviews || []) {
    if (!reviewLabels[review.state] && review.state !== 'DISMISSED') continue
    if ((review.state === 'COMMENTED' || review.state === 'DISMISSED') && !review.body?.trim()) continue
    items.push({
      id: 'review-' + review.id,
      kind: 'review',
      state: review.state,
      label: reviewLabels[review.state] || 'Dismissed review',
      author: review.user,
      date: review.submitted_at,
      html: review.body_html,
      body: review.body || '',
      url: review.html_url,
    })
  }
  for (const comment of status?.comments || []) {
    if (!comment.body?.trim()) continue
    items.push({
      id: 'comment-' + comment.id,
      kind: 'comment',
      label: 'Commented',
      author: comment.user,
      date: comment.created_at,
      html: comment.body_html,
      body: comment.body,
      url: comment.html_url,
    })
  }
  return items.sort((a, b) => Date.parse(a.date) - Date.parse(b.date))
}

export function activityCounts(items) {
  return {
    approved: items.filter((item) => item.state === 'APPROVED').length,
    changes: items.filter((item) => item.state === 'CHANGES_REQUESTED').length,
    comments: items.filter((item) => item.kind === 'comment').length,
  }
}

// Plain text to paste into an agent: the pull request, then each failed check. `details`
// maps a check to the text PullRequestCheckFailure returned for it, when it has a job.
export function failureContext(status, checks, details = new Map(), limit = Infinity) {
  const header = [`Pull request #${status.number}: ${status.title}`, status.url, `Branch: ${status.branch}`]
  const omitted = Math.max(checks.length - limit, 0)
  const blocks = checks.map(
    (check) =>
      details.get(check) ||
      [
        `Check: ${check.workflow ? check.workflow + ' / ' : ''}${check.name}`,
        `Status: ${check.status}`,
        check.url && `URL: ${check.url}`,
      ]
        .filter(Boolean)
        .join('\n'),
  )
  if (omitted) blocks.push(`${omitted} more failing ${omitted === 1 ? 'check' : 'checks'} listed above without logs.`)
  return [header.filter(Boolean).join('\n'), ...blocks].join('\n\n---\n\n')
}

export function activityText(status, item) {
  return [
    `Pull request #${status.number}: ${status.title}`,
    `${item.label} by ${item.author?.login || 'Deleted user'} on ${item.date}`,
    item.url,
    '',
    item.body.trim(),
  ]
    .filter((line) => line !== undefined)
    .join('\n')
}
