import test from 'node:test';
import assert from 'node:assert/strict';
import { execFileSync, spawn } from 'node:child_process';
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';
import { options, validateCase, MAX_RECORD_BYTES, GENERATOR_HASH } from './generate.mjs';

const generator = fileURLToPath(new URL('./generate.mjs', import.meta.url));

test('CLI rejects coercion, missing mandatory budget and incompatible modes', () => {
  for (const args of [
    ['--seed', '1'], ['--count', '18'], ['--profile', 'scale'], ['--stream', '--out', 'x'],
    ['--stream', '--kind', 'oops'], ['--stream', '--profile', 'oops'], ['--unknown'],
    ...['-1', '4294967296', '1.5', 'NaN', '1e3', ' 1', '01', 'Infinity'].map(s => ['--stream', '--seed', s]),
    ...['0', '1', '617', '10000001', 'NaN', '2.5'].map(s => ['--stream', '--count', s]),
  ]) assert.throws(() => options(args), args.join(' '));
  assert.equal(options(['--stream', '--seed', '4294967295', '--count', '618']).seed, 4294967295);
});

test('generation is byte deterministic, validates all goldens and leaves negatives alone', () => {
  const scratch = mkdtempSync(join(tmpdir(), 'stele-oracle-test-'));
  try {
    for (const pass of ['a', 'b']) execFileSync(process.execPath, [generator, '--out', join(scratch, pass)]);
    writeFileSync(join(scratch, 'a', 'invalid.json'), 'hand-written sentinel');
    execFileSync(process.execPath, [generator, '--out', join(scratch, 'a')]);
    assert.equal(readFileSync(join(scratch, 'a', 'invalid.json'), 'utf8'), 'hand-written sentinel');
    for (const kind of ['apply', 'compose', 'transform']) {
      const first = readFileSync(join(scratch, 'a', `${kind}.json`));
      assert.deepEqual(first, readFileSync(join(scratch, 'b', `${kind}.json`)));
      assert.deepEqual(first, readFileSync(new URL(`../../schema/textop/${kind}.json`, import.meta.url)), `${kind}: checked-in fixture drift`);
      const f = JSON.parse(first);
      assert.equal(f.header.generator_hash, GENERATOR_HASH);
      assert.equal(f.header.count, f.cases.length);
      for (const c of f.cases) validateCase(c);
      if (kind === 'apply') {
        assert(f.cases.some(c => c.error === 'length_mismatch'));
        assert.throws(() => validateCase({ ...f.cases[0], want: 'wrong' }));
        // One astral scalar and two ASCII scalars have the SAME UTF-16 length.
        assert.throws(() => validateCase({ id: 'bad-premise', kind, doc: '😀', op: [2], error: 'length_mismatch' }));
      }
    }
  } finally { rmSync(scratch, { recursive: true, force: true }); }
});

test('stream handles slow consumption and propagates closed-pipe errors', async () => {
  const args = [generator, '--stream', '--profile', 'fixtures', '--count', '2000'];
  const child = spawn(process.execPath, args, { stdio: ['ignore', 'pipe', 'pipe'], timeout: 30000 });
  const closed = once(child, 'close'); let pending = '', count = 0, stderr = '';
  child.stdout.setEncoding('utf8');
  child.stderr.on('data', data => { stderr = (stderr + data).slice(-16000); });
  for await (const chunk of child.stdout) {
    pending += chunk.toString('utf8');
    let at;
    while ((at = pending.indexOf('\n')) !== -1) {
      const line = pending.slice(0, at); pending = pending.slice(at + 1);
      assert(Buffer.byteLength(line) < MAX_RECORD_BYTES); const record = JSON.parse(line);
      if (count > 0) assert.equal(record.i, count - 1); count++;
    }
    await new Promise(resolve => setTimeout(resolve, 1));
  }
  assert.deepEqual(await closed, [0, null], stderr); assert.equal(count, 2001); assert.equal(pending, '');
  const broken = spawn(process.execPath, [...args.slice(0, -1), '1000000'], { stdio: ['ignore', 'pipe', 'pipe'], timeout: 30000 });
  const exited = once(broken, 'close'); broken.stderr.resume(); broken.stdout.destroy();
  const [code, signal] = await exited;
  assert.equal(signal, null, 'closed pipe must fail without waiting for the process timeout');
  assert.equal(code, 1, 'pipeline error must propagate through the generator error handler');
});
