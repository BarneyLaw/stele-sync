import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import { collate, reportNightly } from './report.mjs';

test('four failed shards produce one grouped issue, then update that issue', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'stele-report-'));
  try {
    for (let shard = 0; shard < 4; shard++) {
      const path = join(dir, `shard-${shard}`); mkdirSync(path);
      writeFileSync(join(path, 'oracle-failures.ndjson'), JSON.stringify({ header: { seed: shard, generator_hash: 'abc' }, case: { kind: 'transform', i: 5, id: 'transform-1' }, diagnostic: 'transform ap: components differ: <@someone>' }) + '\n');
    }
    const report = collate(dir, 'https://example.com/run', 'failure');
    assert(report.failed); assert.equal((report.body.match(/### transform\/components/g) ?? []).length, 1);
    assert(!report.body.includes('@someone')); assert(report.body.includes('4 shard(s)'));
    const created = [], updated = []; let issues = [];
    const github = { paginate: async () => issues, rest: { issues: {
      listForRepo: () => {}, create: async data => { created.push(data); issues = [{ ...data, number: 42 }]; }, update: async data => { updated.push(data); issues = issues.map(i => i.number === data.issue_number ? { ...i, ...data } : i); },
    }}};
    const context = { serverUrl: 'https://github.com', repo: { owner: 'a', repo: 'b' }, runId: 1, ref: 'refs/heads/main', payload: { repository: { default_branch: 'main' } } };
    const core = { summary: { addRaw() { return this; }, async write() {} } };
    await reportNightly({ github, context, core, directory: dir, result: 'failure' });
    await reportNightly({ github, context, core, directory: dir, result: 'failure' });
    assert.equal(created.length, 1); assert.equal(updated.length, 1); assert.equal(updated[0].issue_number, 42);
    await reportNightly({ github, context: { ...context, ref: 'refs/heads/feature' }, core, directory: dir, result: 'failure' });
    assert.equal(updated.length, 1);
    // A maintainer can rename or close the issue without breaking deduplication.
    issues[0].title = 'Renamed'; issues[0].state = 'closed';
    await reportNightly({ github, context, core, directory: dir, result: 'failure' });
    assert.equal(created.length, 1); assert.equal(updated[1].state, 'open');
    const healthy = join(dir, 'healthy'); mkdirSync(healthy);
    writeFileSync(join(healthy, 'oracle-summary.json'), JSON.stringify({ passed: true, header: { seed: 1 }, coverage: { count: 618 } }));
    await reportNightly({ github, context, core, directory: healthy, result: 'success' });
    const firstHealthy = updated.at(-1).body;
    assert(firstHealthy.includes('**passed**')); assert(firstHealthy.includes('## Last failure')); assert(firstHealthy.includes('components differ'));
    await reportNightly({ github, context, core, directory: healthy, result: 'success' });
    assert.equal(updated.at(-1).body, firstHealthy, 'healthy updates must not accumulate copies');
  } finally { rmSync(dir, { recursive: true, force: true }); }
});

test('missing and broken artifacts are infrastructure failures; success opens no issue', async () => {
  const dir = mkdtempSync(join(tmpdir(), 'stele-report-'));
  try {
    assert(collate(dir, 'https://example.com', 'failure').body.includes('No case artifact'));
    writeFileSync(join(dir, 'oracle-summary.json'), '{');
    assert(collate(dir, 'https://example.com', 'success').failed);
    writeFileSync(join(dir, 'oracle-summary.json'), JSON.stringify({ passed: true, header: { seed: 1 }, coverage: { count: 618 } }));
    assert.equal(collate(dir, 'https://example.com', 'success').failed, false);
    const github = { paginate: async () => [], rest: { issues: { listForRepo() {}, create() { assert.fail('healthy run created issue'); } } } };
    await reportNightly({ github, context: { serverUrl: 'https://github.com', repo: { owner: 'a', repo: 'b' }, runId: 1, ref: 'refs/heads/main', payload: { repository: { default_branch: 'main' } } }, core: { summary: { addRaw() { return this; }, async write() {} } }, directory: dir, result: 'success' });
  } finally { rmSync(dir, { recursive: true, force: true }); }
});
