// Explicit visual fixtures. This module cannot start processes or access native user state.
import { emitPreview } from './api'
import { projectRows } from './appearance'
const id = () => crypto.randomUUID()
const projectID = id(),
  sessionID = id(),
  peerID = id(),
  workID = id(),
  sideID = id()
// Illustrative sign-ins by profile ID ("" is Default); the preview never runs a CLI.
const signedIn = { '': ['codex', 'claude'], [workID]: ['claude'] }
// Profiles with the Orchestra Skill installed for Claude Code in this preview.
const skilled = new Set(['', workID])
const previewUpdate = {
  current: '0.1.0',
  supported: true,
  installable: true,
  checkedAt: new Date().toISOString(),
  available: {
    version: '0.2.0',
    name: 'Orchestra 0.2.0',
    url: 'https://github.com/theorchestradev/orchestra/releases',
    prerelease: false,
    notes:
      "## What's Changed\n\n" +
      Array.from(
        { length: 12 },
        (_, i) =>
          `* Example change ${i + 1} with a long description that wraps across lines in the release notes by @example in #${60 + i}`,
      ).join('\n'),
  },
}
let state = {
  version: 1,
  selectedProjectID: projectID,
  preferences: {
    appearance: 'dark',
    theme: 'system',
    terminalFontFamily: '',
    terminalFontSize: 14,
    terminalFontSmoothing: true,
    terminalPaddingX: 12,
    terminalPaddingY: 8,
    sidebarWidth: 280,
    diffPaneWidth: 440,
    diffPaneOpen: false,
    arcMode: false,
    defaultAgent: 'claude',
    startupView: 'last',
    confirmQuit: true,
    confirmSessionClose: true,
    gitHubRefreshInterval: 0,
    deliverRequests: false,
    translucent: false,
    windowOpacity: 78,
    updateChecksOff: false,
    hideOpenInButton: false,
    hideUsageStatus: false,
    usageDetailed: false,
    keepLocalMainUpdated: false,
    betaUpdates: false,
  },
  projects: [
    {
      id: projectID,
      name: 'Orchestra',
      directory: '/tmp/orchestra-visual-fixture',
      isExpanded: true,
      repository: 'example/orchestra',
      hasScratchpad: true,
      scratchpadText: '',
      scratchpadRevision: 0,
      showsScratchpad: true,
      showsGitHub: false,
      selectedSessionID: sessionID,
      profileID: workID,
      sessions: [
        {
          id: sessionID,
          name: 'Claude Code 1',
          agent: 'claude',
          agentSessionID: sessionID,
          hasLaunched: true,
          worktreeBranch: 'orchestra/claude-' + sessionID.slice(0, 8),
          worktreeDirectory: '/tmp/orchestra-preview-worktrees/' + sessionID.slice(0, 8),
          restored: true,
          running: false,
          activity: 'waiting',
          profileID: workID,
        },
        {
          id: peerID,
          name: 'Codex 1',
          agent: 'codex',
          hasLaunched: true,
          restored: true,
          running: false,
          activity: 'working',
          profileID: sideID,
        },
      ],
    },
  ],
  profiles: [
    {
      id: workID,
      name: 'Work',
      emoji: '💼',
      folders: {
        claude: '~/.orchestra/profiles/work/claude',
        codex: '~/.orchestra/profiles/work/codex',
      },
    },
    {
      id: sideID,
      name: 'Side Projects and Open Source Contributions Account',
      folders: {
        claude: '~/.orchestra/profiles/side-projects-and-open-source-contribut/claude',
        codex: '~/.orchestra/profiles/side-projects-and-open-source-contribut/codex',
      },
    },
  ],
  runs: [],
}
const clone = () => structuredClone(state),
  changed = () => {
    emitPreview('workspace', clone())
    return clone()
  },
  project = (id) => state.projects.find((p) => p.id === id),
  session = (id) => state.projects.flatMap((p) => p.sessions).find((s) => s.id === id)
const item = {
  number: 42,
  title: 'Preserve terminal sessions when switching projects',
  state: 'open',
  html_url: 'https://github.com/example/orchestra/issues/42',
  updated_at: new Date().toISOString(),
  created_at: new Date().toISOString(),
  user: { login: 'contributor' },
  labels: [{ name: 'enhancement', color: '58a6ff' }],
  assignees: [],
}
const labels = [
  { name: 'enhancement', color: '58a6ff' },
  { name: 'bug', color: 'e45656' },
  { name: 'needs-triage', color: 'd4c5f9' },
  { name: 'a very long label name for compact window verification', color: 'e5c07b' },
]
const assignees = [{ login: 'contributor' }, { login: 'reviewer' }]
const githubItems = [
  item,
  { ...structuredClone(item), number: 44, title: 'Keep session processes alive across navigation', labels: [] },
  {
    ...structuredClone(item),
    number: 43,
    title: 'Update session activity indicators',
    pull_request: { html_url: 'https://github.com/example/orchestra/pull/43' },
    labels: [],
  },
]
// A fake Changes pane: every status, a long path, a binary file and a file too large to show.
const previewFiles = [
  {
    path: 'frontend/src/components/a-very-long-folder-name-for-compact-window-verification/SessionDiff.vue',
    status: 'A',
    additions: 4,
    deletions: 0,
    untracked: true,
    patch: '@@ -0,0 +1,4 @@\n+<script setup>\n+// Changes pane fixture\n+</script>\n+<template><aside /></template>\n',
  },
  {
    path: 'internal/core/session.go',
    status: 'M',
    additions: 3,
    deletions: 2,
    patch:
      '@@ -231,7 +231,8 @@ func (s *Store) StartSession(id string) {\n \tdirectory := p.Directory\n-\tif sess.WorktreeDirectory != "" {\n-\t\tdirectory = sess.WorktreeDirectory\n+\tif worktree := sess.WorktreeDirectory; worktree != "" {\n+\t\t// Missing worktrees never fall back to the project folder.\n+\t\tdirectory = worktree\n \t}\n \treturn directory\n',
  },
  { path: 'docs/notes.md', oldPath: 'NOTES.md', status: 'R', additions: 0, deletions: 0 },
  { path: 'legacy.txt', status: 'D', additions: 0, deletions: 1, patch: '@@ -1 +0,0 @@\n-Remove me\n' },
  { path: 'frontend/public/preview.png', status: 'M', additions: 0, deletions: 0, binary: true },
  { path: 'package-lock.json', status: 'M', additions: 4200, deletions: 3100, truncated: true },
]
// Mirrors Store.moveRowLocked: index counts the Scratchpad among the project's sessions.
function moveRow(p, target, index) {
  const rows = projectRows(p),
    from = rows.findIndex((r) => r.id === target)
  rows.splice(Math.max(0, Math.min(index, rows.length - 1)), 0, ...rows.splice(from, 1))
  p.sessions = rows.filter((r) => !r.scratchpad)
  if (p.hasScratchpad) p.scratchpadIndex = rows.findIndex((r) => r.scratchpad)
  return changed()
}
let openRouterConnected = false
// Themes made with Duplicate as Custom Theme during this preview.
const previewThemes = []
const openRouterAccount = () =>
  openRouterConnected
    ? {
        configured: true,
        balance: 74.75,
        credits: 100.5,
        usage: 25.75,
        activityFrom: '2026-09-07',
        activityTo: '2026-10-06',
        models: [
          {
            model: 'anthropic/claude-sonnet-4.5',
            usage: 18.5,
            byok: 0,
            requests: 425,
            promptTokens: 1240500,
            completionTokens: 310200,
            reasoningTokens: 12000,
          },
          {
            model: 'openai/gpt-5.4',
            usage: 5.25,
            byok: 2.7,
            requests: 180,
            promptTokens: 620000,
            completionTokens: 98000,
            reasoningTokens: 15000,
          },
          {
            model: 'google/gemini-2.5-flash',
            usage: 0.000014,
            byok: 0,
            requests: 2,
            promptTokens: 120,
            completionTokens: 10,
            reasoningTokens: 0,
          },
        ],
        checkedAt: Math.floor(Date.now() / 1000),
      }
    : { configured: false }
export const backend = {
  Track: () => {},
  TelemetryStatus: () => ({ available: true, reason: '', debug: true }),
  SetTelemetryConsent: (enabled) => {
    state.preferences.telemetryEnabled = enabled
    state.preferences.telemetryChoiceMade = true
    return changed()
  },
  OpenRouterUsage: () => openRouterAccount(),
  SetOpenRouterKey: (key) => {
    if (key !== 'orchestra-preview-management-key')
      throw Error('Use orchestra-preview-management-key in this synthetic preview.')
    openRouterConnected = true
    return openRouterAccount()
  },
  RemoveOpenRouterKey: () => {
    openRouterConnected = false
  },
  AgentUsage: (refresh, profileID = '') => {
    const checkedAt = Math.floor(Date.now() / 1000)
    if (profileID)
      return ['codex', 'claude'].map((agent) => ({
        agent,
        checkedAt,
        windows: signedIn[profileID]?.includes(agent)
          ? [{ label: 'Weekly', remaining: 41, resetsAt: checkedAt + 86400 }]
          : [],
        message: signedIn[profileID]?.includes(agent) ? '' : 'Log in to this profile to see subscription limits.',
      }))
    return [
      {
        agent: 'codex',
        checkedAt,
        windows: [{ label: 'Weekly', remaining: 97, resetsAt: checkedAt + 6 * 86400 + 10 * 3600 }],
        resetCredits: 2,
      },
      {
        agent: 'claude',
        checkedAt,
        windows: [
          { label: 'Session · 5h', remaining: 100 },
          { label: 'Weekly', remaining: 96, resetsAt: checkedAt + 2 * 86400 + 6 * 3600 },
          { label: 'Weekly · Fable', remaining: 100, resetsAt: checkedAt + 2 * 86400 + 6 * 3600 },
        ],
      },
    ]
  },
  ConfirmQuit: () => console.debug('Preview: quit confirmed.'),
  // Illustrative release with long notes for visual QA; the preview never contacts GitHub.
  UpdateStatus: () => previewUpdate,
  InstallUpdate: async (version, restart) => {
    emitPreview('update', { ...previewUpdate, preparing: true })
    await new Promise((r) => setTimeout(r, 1200))
    previewUpdate.staged = version
    if (restart) console.debug('Preview: the app would restart into', version)
    return previewUpdate
  },
  CancelUpdateInstall: () => {},
  CheckForUpdates: async () => {
    emitPreview('update', { ...previewUpdate, checking: true })
    await new Promise((r) => setTimeout(r, 600))
    return previewUpdate
  },
  SystemPalettes: () => null,
  // Illustrative apps with long names and no system icons, as on Linux.
  ProjectIcons: () => ({}),
  OpenInApps: () => [
    { id: 'vscode', name: 'VS Code', kind: 'editor' },
    { id: 'zed', name: 'Zed', kind: 'editor' },
    { id: 'intellij', name: 'IntelliJ IDEA Ultimate Edition With A Very Long Name', kind: 'editor' },
    { id: 'ghostty', name: 'Ghostty', kind: 'terminal' },
    { id: 'fork', name: 'Fork', kind: 'git' },
    { id: 'files', name: 'File Manager', kind: 'files' },
  ],
  OpenSessionIn: () => {},
  TerminalFonts: () => ['Courier New', 'JetBrains Mono', 'JetBrainsMono Nerd Font Mono Extra Condensed Light', 'Menlo'],
  // A browser cannot blur the desktop, so an illustrative wallpaper stands in for visual QA.
  WindowTranslucency: (on) => {
    document.body.style.background = on ? 'linear-gradient(135deg, #f5a524, #7c3aed 45%, #0ea5e9)' : ''
    return on
  },
  // Illustrative custom theme with a long name and one skipped folder for visual QA.
  CustomThemes: () => ({
    directory: '~/.orchestra/themes',
    themes: [
      ...previewThemes,
      {
        id: 'custom:harbor-lights',
        name: 'Harbor Lights at Dusk with an Unusually Long Theme Name',
        light: ['#F6F8F7', '#ECF1EF', '#FFFFFF', '#1D2B2A', '#5B6B69', '#D3DDDA', '#0F7B6C', '#D6ECE7'],
        dark: ['#0F1A1C', '#132124', '#1A2C30', '#DCE7E4', '#93A8A4', '#2A3F43', '#3CC7B0', '#1D3D3B'],
      },
    ],
    errors: ['broken-theme: "dark" needs 8 colors, found 6.'],
  }),
  DuplicateTheme: (name, light, dark, ansi) => {
    const theme = { id: 'custom:' + name.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, '-'), name, light, dark, ansi }
    previewThemes.push(theme)
    return theme
  },
  RevealCustomTheme: (id) => console.debug('Preview: reveal ' + id),
  Workspace: clone,
  ChooseFolder: () => '/tmp/orchestra-visual-fixture',
  CreateProject: (name, directory, description = '') => {
    const p = {
      id: id(),
      name,
      description: description.trim(),
      directory,
      isExpanded: true,
      sessions: [],
      hasScratchpad: false,
      scratchpadText: '',
      scratchpadRevision: 0,
      repository: '',
    }
    state.projects.push(p)
    state.selectedProjectID = p.id
    return changed()
  },
  Select: (pid, destination) => {
    state.selectedProjectID = pid
    const p = project(pid)
    if (destination !== 'project') {
      p.showsScratchpad = destination === 'scratchpad'
      p.showsGitHub = destination === 'github'
      if (p.showsScratchpad) {
        p.hasScratchpad = true
        p.isExpanded = true
      } else if (!p.showsGitHub) p.selectedSessionID = destination
    }
    return changed()
  },
  EditProject: (pid, name, description, emoji = '') => {
    Object.assign(project(pid), { name: name.trim(), description: description.trim(), emoji: emoji.trim() })
    return changed()
  },
  Rename: (target, name) => {
    ;(project(target) || session(target)).name = name
    return changed()
  },
  RenameScratchpad: (pid, name) => {
    project(pid).scratchpadName = name.trim()
    return changed()
  },
  SetScratchpadMode: (pid, mode) => {
    project(pid).scratchpadMode = mode === 'read' ? 'read' : ''
    return changed()
  },
  DeleteScratchpad: (pid) => {
    Object.assign(project(pid), {
      hasScratchpad: false,
      showsScratchpad: false,
      scratchpadName: '',
      scratchpadMode: '',
      scratchpadText: '',
      scratchpadRevision: 0,
      scratchpadIndex: 0,
    })
    return changed()
  },
  RemoveProject: (pid) => {
    const p = project(pid),
      ids = new Set(p.sessions.map((s) => s.id))
    for (const run of state.runs)
      if (['queued', 'running'].includes(run.status) && (ids.has(run.fromSessionID) || ids.has(run.toSessionID)))
        run.status = 'interrupted'
    state.projects = state.projects.filter((p) => p.id !== pid)
    if (state.selectedProjectID === pid) state.selectedProjectID = state.projects[0]?.id || ''
    return changed()
  },
  MoveProject: (pid, index) => {
    const from = state.projects.findIndex((p) => p.id === pid)
    state.projects.splice(Math.max(0, Math.min(index, state.projects.length - 1)), 0, ...state.projects.splice(from, 1))
    return changed()
  },
  MoveSession: (target, index) =>
    moveRow(
      state.projects.find((p) => p.sessions.some((s) => s.id === target)),
      target,
      index,
    ),
  MoveScratchpad: (pid, index) => moveRow(project(pid), 'scratchpad', index),
  ToggleProject: (pid) => {
    const p = project(pid)
    p.isExpanded = !p.isExpanded
    return changed()
  },
  SaveScratchpad: (pid, text) => {
    const p = project(pid)
    if (p.scratchpadText !== text) p.scratchpadRevision++
    p.scratchpadText = text
    return p.scratchpadRevision
  },
  SetPreferences: (preferences) => {
    state.preferences = structuredClone(preferences)
    return changed()
  },
  CanCreateWorktree: (pid) => !/no-git/.test(project(pid).directory) || !!project(pid).gitInitialized,
  DetectRepository: (pid) => {
    const p = project(pid)
    if (!p.repository) p.repository = 'example/' + p.name.toLowerCase().replace(/\W+/g, '-')
    return changed()
  },
  AgentSkillStatus: async () => {
    await new Promise((r) => setTimeout(r, 300))
    const found = ['codex', 'claude', 'pi']
    return {
      node: true,
      detected: true,
      agents: ['codex', 'claude', 'pi', 'vibe', 'opencode', 'antigravity', 'grok', 'cline', 'omp'].map((id) => ({
        id,
        installed: found.includes(id),
      })),
    }
  },
  AgentSkillCommand: (ids) => {
    const keys = { claude: 'claude-code', antigravity: 'antigravity-cli', vibe: 'mistral-vibe' }
    return (
      'npx --yes skills@1.7.2 add https://github.com/theorchestradev/orchestra --skill orchestra --global --yes' +
      ids.map((id) => ' --agent ' + (keys[id] || id)).join('')
    )
  },
  InstallAgentSkill: async (ids) => {
    await new Promise((r) => setTimeout(r, 800))
    return { exitCode: 0, output: 'Preview: the skills CLI is not run.\nSelected agents: ' + ids.join(', ') }
  },
  CreateProfile: (name, emoji) => {
    if (!name.trim()) throw Error('Enter a profile name.')
    const slug =
      name
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '') || 'profile'
    state.profiles.push({
      id: id(),
      name: name.trim(),
      emoji,
      folders: {
        claude: `~/.orchestra/profiles/${slug}/claude`,
        codex: `~/.orchestra/profiles/${slug}/codex`,
      },
    })
    return changed()
  },
  EditProfile: (profileID, name, emoji) => {
    if (!name.trim()) throw Error('Enter a profile name.')
    Object.assign(
      state.profiles.find((p) => p.id === profileID),
      { name: name.trim(), emoji },
    )
    return changed()
  },
  RemoveProfile: (profileID) => {
    state.profiles = state.profiles.filter((p) => p.id !== profileID)
    for (const p of state.projects) if (p.profileID === profileID) p.profileID = ''
    return changed()
  },
  SetProjectProfile: (pid, profileID) => {
    project(pid).profileID = profileID
    return changed()
  },
  ProfileLogins: async (profileID) => {
    await new Promise((r) => setTimeout(r, 400))
    const profile = state.profiles.find((p) => p.id === profileID)
    return ['codex', 'claude'].map((agent) => ({
      agent,
      folder: profile?.folders?.[agent] || '',
      loggedIn: !!signedIn[profileID]?.includes(agent),
      // Codex skills are shared; Claude Code needs the skill in each profile.
      skill: agent === 'codex' || skilled.has(profileID),
    }))
  },
  InstallProfileSkill: async (profileID) => {
    await new Promise((r) => setTimeout(r, 800))
    skilled.add(profileID)
    return { exitCode: 0, output: 'Preview: the skills CLI is not run.' }
  },
  LogIn: (pid, profileID, agent) => {
    ;(signedIn[profileID] ||= []).push(agent)
    const name = { claude: 'Claude Code', codex: 'Codex' }[agent]
    const profile = state.profiles.find((p) => p.id === profileID)
    backend.CreateSession(
      pid,
      'terminal',
      `Log In · ${name} · ${profile?.name || 'Default'}`,
      '',
      false,
      null,
      '',
      '',
      profileID,
    )
    return changed()
  },
  AgentModels: async (agent) => {
    const levels = ['low', 'medium', 'high', 'xhigh', 'max']
    await new Promise((r) => setTimeout(r, 300))
    return (
      {
        claude: [
          { id: 'fable', name: 'Fable', efforts: levels },
          { id: 'opus', name: 'Opus', efforts: levels },
          { id: 'sonnet', name: 'Sonnet', efforts: levels },
          { id: 'haiku', name: 'Haiku', efforts: [] },
        ],
        codex: [
          { id: 'gpt-6.1-sol', name: 'GPT-6.1-Sol', efforts: [...levels, 'ultra'] },
          {
            id: 'gpt-6-astra-with-a-very-long-preview-model-name',
            name: 'GPT-6-Astra with a very long preview model name for layout checks',
            efforts: ['low', 'medium', 'high'],
          },
        ],
        opencode: [{ id: 'opencode/big-pickle', name: 'opencode/big-pickle', efforts: [] }],
        grok: [{ id: 'grok-4.7', name: 'grok-4.7', efforts: [] }],
        omp: [
          {
            id: 'openrouter/~anthropic/claude-opus-latest',
            name: 'Claude Opus Latest',
            efforts: ['low', 'medium', 'high', 'xhigh', 'max'],
          },
        ],
        antigravity: [{ id: 'gemini-3.8-flash-high', name: 'Gemini 3.8 Flash (High)', efforts: [] }],
      }[agent] ||
      (agent === 'pi' ? Promise.reject('Could not list models. Check that Pi is installed and signed in.') : [])
    )
  },
  // Preview folders named *no-git* disable worktrees in New Session and offer Initialize Git.
  InitializeGitRepository: async (pid) => {
    await new Promise((r) => setTimeout(r, 600))
    const p = project(pid)
    if (p.gitInitialized || !/no-git/.test(p.directory))
      return Promise.reject('The project folder is already in the Git repository at ' + p.directory + '.')
    p.gitInitialized = true
    return changed()
  },
  CreateSession: (pid, agent, name, prompt, createWorktree, gitHubItem, model, effort, profileID = '') => {
    if (createWorktree && agent !== 'terminal' && /no-git/.test(project(pid).directory))
      return Promise.reject(
        project(pid).gitInitialized
          ? 'Commit the repository before creating a worktree: git: fatal: Needed a single revision'
          : 'A worktree requires a Git repository: git: fatal: not a git repository (or any of the parent directories): .git',
      )
    const p = project(pid),
      sess = {
        id: id(),
        name:
          name
            .split(/\s+/)
            .filter(Boolean)
            .join(' ')
            .replace(/^(.{59}).{2,}$/su, (_, s) => s.trim() + '…') ||
          {
            claude: 'Claude Code',
            vibe: 'Mistral Vibe',
            antigravity: 'Antigravity CLI',
            grok: 'Grok Build',
            cline: 'Cline',
            omp: 'omp',
            opencode: 'OpenCode',
            pi: 'Pi',
            codex: 'Codex',
            terminal: 'Terminal',
          }[agent] +
            ' ' +
            (p.sessions.filter((s) => s.agent === agent).length + 1),
        agent,
        activity: 'unknown',
        hasLaunched: false,
        initialPrompt: prompt,
      }
    if (gitHubItem && agent !== 'terminal') sess.gitHubItem = gitHubItem
    if (profileID) sess.profileID = profileID
    if (model) sess.model = model
    if (effort) sess.effort = effort
    if (createWorktree) {
      sess.worktreeBranch = 'orchestra/' + agent + '-' + sess.id.slice(0, 8)
      sess.worktreeDirectory = '/tmp/orchestra-preview-worktrees/' + sess.id
    }
    p.sessions.push(sess)
    p.selectedSessionID = sess.id
    p.showsScratchpad = p.showsGitHub = false
    p.isExpanded = true
    state.selectedProjectID = pid
    const result = changed()
    // The preview's local main is always a few commits behind origin.
    if (
      createWorktree &&
      agent !== 'terminal' &&
      !state.preferences.keepLocalMainUpdated &&
      !p.localMainNoticeDismissed
    )
      setTimeout(() => emitPreview('local-main-behind', { projectID: pid, branch: 'main', behind: 4 }))
    return result
  },
  KeepLocalMainUpToDate: () => {
    state.preferences.keepLocalMainUpdated = true
    return changed()
  },
  DismissLocalMainNotice: (pid) => {
    project(pid).localMainNoticeDismissed = true
    return changed()
  },
  StartSession: (target) => {
    const s = session(target)
    s.running = true
    s.restored = false
    s.hasLaunched = true
    s.activity = 'waiting'
    changed()
    const text = 'Visual preview — no agent process is running.\r\n'
    const bytes = new TextEncoder().encode(text)
    return { id: target, start: 0, end: bytes.length, data: btoa(String.fromCharCode(...bytes)) }
  },
  SessionDiff: (target) => {
    const s = session(target)
    if (s.agent === 'terminal')
      return {
        files: [],
        message: 'Terminal sessions don’t track changes. Agent sessions show what they changed here.',
      }
    const files = s.worktreeDirectory ? previewFiles : previewFiles.slice(1, 2)
    return {
      scope: s.worktreeDirectory ? 'worktree' : 'folder',
      directory: s.worktreeDirectory || project(state.selectedProjectID)?.directory,
      base: s.worktreeDirectory ? '3f9c2a1b7d4e5f60718293a4b5c6d7e8f9012345' : 'HEAD',
      files: structuredClone(files),
      additions: files.reduce((n, f) => n + f.additions, 0),
      deletions: files.reduce((n, f) => n + f.deletions, 0),
      truncated: files.some((f) => f.truncated),
    }
  },
  TerminalSnapshot: (target) => ({ id: target, start: 0, end: 0, data: '' }),
  WriteTerminal: () => {},
  ResizeTerminal: () => {},
  CloseSession: (target) => {
    for (const p of state.projects) {
      const i = p.sessions.findIndex((s) => s.id === target)
      if (i >= 0 && i < (p.scratchpadIndex || 0)) p.scratchpadIndex--
      p.sessions = p.sessions.filter((s) => s.id !== target)
      if (p.selectedSessionID === target) p.selectedSessionID = p.sessions[0]?.id || ''
    }
    return changed()
  },
  GitHubItems: (pid, status) => {
    console.debug('Preview: GitHub refresh', pid)
    return structuredClone(githubItems.filter((i) => status === 'all' || i.state === status))
  },
  GitHubDetail: (_pid, number, pull) => ({
    detail: {
      body: 'Keep each project’s processes and terminal scrollback alive when navigating between sessions.\n\n```\nSwitch projects → return to the same conversation\n```',
      body_html:
        '<p>Keep each project’s processes and terminal scrollback alive when navigating between sessions.</p><pre><code>Switch projects → return to the same conversation</code></pre>',
      ...structuredClone(githubItems.find((i) => i.number === number)),
      ...(pull
        ? { head: { label: 'feature/status' }, base: { label: 'main' }, changed_files: 3, additions: 52, deletions: 12 }
        : {}),
    },
    comments: [
      {
        id: 1,
        user: { login: 'reviewer' },
        created_at: new Date().toISOString(),
        body_html: '<p>The session owns the process and the view only renders it.</p>',
      },
    ],
  }),
  GitHubMetadataOptions: (_pid, field) => structuredClone(field === 'labels' ? labels : assignees),
  UpdateGitHubMetadata: (_pid, number, _pull, field, values) => {
    const target = githubItems.find((i) => i.number === number)
    if (field === 'body') {
      const escape = (text) =>
        text.replace(/[&<>"]/g, (c) => `&${{ '&': 'amp', '<': 'lt', '>': 'gt', '"': 'quot' }[c]};`)
      target.body = values[0]
      target.body_html =
        values[0] &&
        values[0]
          .split(/\n{2,}/)
          .map((p) => `<p>${escape(p)}</p>`)
          .join('')
      target.updated_at = new Date().toISOString()
      return
    }
    if (field === 'state') target.state_reason = values[0] === 'open' ? 'reopened' : values[1] || 'completed'
    target[field] =
      field === 'state'
        ? values[0]
        : (field === 'labels' ? labels : assignees).filter((o) =>
            values.includes(o[field === 'labels' ? 'name' : 'login']),
          )
    target.updated_at = new Date().toISOString()
  },
  SendRequest: (from, to, prompt) => {
    state.runs.push({
      id: id(),
      projectID: state.selectedProjectID,
      fromSessionID: from,
      toSessionID: to,
      prompt,
      status: 'queued',
      response: '',
      createdAt: Date.now() / 1000 - 978307200,
    })
    return changed()
  },
  CancelRequest: (target) => {
    state.runs.find((r) => r.id === target).status = 'cancelled'
    return changed()
  },
  ExecuteRequest: (target) => {
    const run = state.runs.find((r) => r.id === target)
    run.status = 'running'
    const p = project(run.projectID)
    p.showsScratchpad = p.showsGitHub = false
    p.selectedSessionID = run.toSessionID
    return changed()
  },
}
