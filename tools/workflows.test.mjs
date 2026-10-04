import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { spawnSync } from 'node:child_process';

const read = name => readFileSync(new URL(`../.github/${name}`, import.meta.url), 'utf8');
const ci = read('workflows/ci.yml');
const core = read('workflows/core.yml');
const nightly = read('workflows/nightly.yaml');

test('ignored CI evidence is explicitly uploadable without uploading the whole cache', () => {
  const uploads = core.split('uses: actions/upload-artifact@v4').slice(1);
  const evidence = uploads.filter(step => step.split(/\n  [a-z]+:/)[0].includes('.cache/core-ci/'));
  assert.equal(evidence.length, 2);
  for (const step of evidence) assert.match(step, /include-hidden-files: true/);
  assert.match(core, /path: \.cache\/core-ci\/plan.json/);
  assert.match(core, /\.cache\/core-ci\/\*\*\/coverage.out/);
  assert.doesNotMatch(core, /path: \.cache\s*$/m);
});

test('all existing required contexts survive, and verification gates the discovered matrix', () => {
  const gates = ci.match(/gate: \[([^\]]+)\]/)[1].split(',').map(s => s.trim());
  const contexts = [...gates.map(g => `M0 / ${g}`), 'M0 / verification', 'M0 / worker'].sort();
  assert.deepEqual(contexts, JSON.parse(read('main-protection.json')).required_status_checks.contexts.sort());
  const verification = ci.slice(ci.indexOf('  verification:'), ci.indexOf('  worker:'));
  assert.match(verification, /needs: core\n    if: always\(\)/);
  assert.match(verification, /CORE_RESULT: \$\{\{ needs.core.result \}\}/);
  assert.match(verification, /run: test "\$CORE_RESULT" = success/);
  assert.match(verification, /node tools\/tasks.mjs fixtures-check/);
  assert.match(ci, /uses: \.\/\.github\/workflows\/core.yml\n    with:\n      mode: pr/);
  assert.match(nightly, /uses: \.\/\.github\/workflows\/core.yml\n    with:\n      mode: nightly/);
  assert.doesNotMatch(nightly, /gate: \[[^\]]*fuzz-short/);
  assert.match(nightly, /gate: \[[^\]]*textop-perf/);
});

test('workflow result script rejects failed/cancelled children and permits only genuinely empty matrices', () => {
  const section = core.slice(core.indexOf('      - name: Require every discovered check to pass'));
  const script = section.match(/        run: \|\n([\s\S]+)$/)[1].replace(/^          /gm, '');
  const bash = process.platform === 'win32' ? 'C:/Program Files/Git/bin/bash.exe' : 'bash';
  for (const [hasChecks, hasFuzz] of [['true', 'true'], ['true', 'false'], ['false', 'true'], ['false', 'false'], ['', '']]) {
    for (const discovery of ['success', 'failure', 'cancelled', 'skipped']) {
      for (const child of ['success', 'failure', 'cancelled', 'skipped']) {
        const expectedFuzz = hasFuzz === 'true' ? 'success' : 'skipped';
        for (const failingJob of ['CHECKS', 'FUZZ']) {
          const env = { ...process.env, DISCOVERY: discovery, HAS_CHECKS: hasChecks, HAS_FUZZ: hasFuzz,
            CHECKS: hasChecks === 'true' ? 'success' : 'skipped', FUZZ: expectedFuzz, [failingJob]: child };
          const result = spawnSync(bash, ['-e', '-c', script], { env, encoding: 'utf8' });
          assert.ifError(result.error);
          const expected = discovery === 'success' && hasChecks !== '' && hasFuzz !== '' &&
            env.CHECKS === (hasChecks === 'true' ? 'success' : 'skipped') && env.FUZZ === expectedFuzz;
          assert.equal(result.status === 0, expected, JSON.stringify({ discovery, hasChecks, hasFuzz, failingJob, child }));
        }
      }
    }
  }
});
