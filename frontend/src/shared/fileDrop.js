// Files dropped on a terminal are typed as shell words, the way Terminal and iTerm do. Plain
// paths stay readable; anything else is single-quoted, which also covers spaces and newlines.
const plain = /^[\w@%+=:,./-]+$/u

export const shellWord = (path) => (plain.test(path) ? path : `'${path.replaceAll("'", `'\\''`)}'`)

// The trailing space lets the next word follow without typing one.
export const shellPaths = (paths = []) => (paths.length ? paths.map(shellWord).join(' ') + ' ' : '')
