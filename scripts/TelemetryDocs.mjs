import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

const schema = JSON.parse(readFileSync(new URL('../internal/telemetry/events.json', import.meta.url), 'utf8'))
const path = new URL('../docs/privacy.md', import.meta.url)
const document = readFileSync(path, 'utf8')
const table = [
  '| Event | Properties and allowed values |',
  '| --- | --- |',
  ...Object.entries(schema).map(([event, properties]) =>
    `| \`${event}\` | ${Object.entries(properties).map(([key, rule]) =>
      `\`${key}\`: ${rule.values ? rule.values.map(value => `\`${value}\``).join(', ') : rule.type === 'boolean' ? 'boolean' : 'release version'}`,
    ).join('; ')} |`,
  ),
].join('\n')
const expected = document.replace(/<!-- telemetry-events:start -->[\s\S]*?<!-- telemetry-events:end -->/,
  `<!-- telemetry-events:start -->\n${table}\n<!-- telemetry-events:end -->`)
if (process.argv[1] === fileURLToPath(import.meta.url) && process.argv.includes('--write')) writeFileSync(path, expected)
else if (document !== expected) throw Error('Privacy docs differ from the analytics allowlist. Run node scripts/TelemetryDocs.mjs --write.')
