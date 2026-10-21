import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'

const script = readFileSync(new URL('./package-macos.sh', import.meta.url), 'utf8')
const helper = script.match(/^staple\(\) \{\n[\s\S]*?^\}/m)?.[0]
assert.ok(helper, 'stapling helper exists')

for (const [failures, error, expectedStatus, expectedCalls] of [
  [0, 68, 0, 1],
  [2, 68, 0, 3],
  [9, 68, 68, 3],
  [1, 65, 65, 1],
]) {
  const result = spawnSync('sh', [], {
    encoding: 'utf8',
    input: `
calls=0
xcrun() { calls=$((calls + 1)); if [ "$calls" -le ${failures} ]; then return ${error}; fi; }
sleep() { :; }
${helper}
staple artifact
status=$?
printf '%s\\n' "$calls"
exit "$status"
`,
  })
  assert.equal(result.status, expectedStatus, result.stderr)
  assert.equal(Number(result.stdout.trim()), expectedCalls)
}
console.log('PASS: macOS stapling retries error 68, stops after three attempts and preserves other errors')
