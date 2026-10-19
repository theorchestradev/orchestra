// A project's emoji is stored apart from its name, so sorting, search and the CLI keep the plain name.
export const projectTitle = (p) => (p?.emoji ? p.emoji + ' ' + p.name : p?.name || '')

// Arc Mode spaces show the emoji, or else the name's first letter.
export const spaceSymbol = (p) => p.emoji || Array.from(p.name.trim())[0]?.toUpperCase() || '•'

// New Session offers Initialize Git Repository only for this error from createSessionWorktree.
export const needsGitRepository = (error) =>
  /^(Error: )?A worktree requires a Git repository\b/.test(String(error ?? ''))
