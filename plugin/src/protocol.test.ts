import { describe, expect, it } from 'vitest';
import clientFixtures from '../../schema/protocol/c2s/valid.json';
import serverFixtures from '../../schema/protocol/s2c/valid.json';
import invalidFixtures from '../../schema/protocol/s2c/invalid.json';
import limits from '../../schema/protocol/limits.json';
import { codes, decodeServer, encodeClient, ProtocolError, uuidv7 } from './core/protocol';
import type { ClientMsg, Code, QueryCode } from './core/protocol';
import { annotation } from './core/protocol/annotation';
import { parse, unicode } from './core/protocol/validate';

const id = '019a1b2c-0000-7000-8000-000000000abc';
describe('M4 directional fixtures', () => {
  for (const f of clientFixtures) it(`encodes ${f.name}`, () => {
    // Fixtures are intentional unknown inputs: encodeClient validates at runtime.
    expect(JSON.parse(encodeClient(f.frame as ClientMsg))).toEqual(f.frame);
  });
  for (const f of serverFixtures) it(`decodes ${f.name}`, () => {
    expect(decodeServer(JSON.stringify(f.frame))).toEqual(f.frame);
    const withExtension = { ...f.frame, future: { nested: '😀' } };
    expect(decodeServer(JSON.stringify(withExtension))).toEqual(withExtension);
  });
  for (const f of invalidFixtures.Cases) it(`rejects ${f.Name}`, () => {
    try { decodeServer(f.Frame); throw new Error('accepted invalid fixture'); }
    catch (error) { expect(error).toBeInstanceOf(ProtocolError); expect((error as ProtocolError).reason).toBe(f.Reason); }
  });
});
it('keeps annotation content byte exact through encoding and decoding', () => {
  const raw = '{"strokes":[],"n":1.0,"s":"\\u0061<>&"}';
  const m: ClientMsg = { t: 'submit', client_op_id: id, kind: 'create', file_id: id, path: 'x.pdf.annot.json', class: 'B', content: { sidecar: raw } };
  const encoded: unknown = JSON.parse(encodeClient(m)); expect(encoded).toEqual(m);
  const fixture = serverFixtures.find(f => f.name === 'change_annot'); expect(fixture).toBeDefined();
  if (!fixture) throw new Error('missing fixture');
  expect(decodeServer(JSON.stringify(fixture.frame))).toEqual(fixture.frame);
});
it('checks UUIDv7 time, entropy, version, variant and lowercase spelling', () => {
  expect(uuidv7(0, new Uint8Array(10))).toBe('00000000-0000-7000-8000-000000000000');
  expect(uuidv7(2 ** 48 - 1, new Uint8Array(10).fill(255))).toBe('ffffffff-ffff-7fff-bfff-ffffffffffff');
  for (const time of [-1, 0.5, 2 ** 48, NaN, Infinity]) expect(() => uuidv7(time, new Uint8Array(10))).toThrow(ProtocolError);
  expect(() => uuidv7(0, new Uint8Array(9))).toThrow(ProtocolError);
});
it('rejects unsafe values and lone surrogates before emitting client bytes', () => {
  for (const value of [null, -1, 0.5, 2 ** 32, 2 ** 53, NaN, Infinity]) {
    expect(() => encodeClient({ t: 'pong', n: value } as ClientMsg)).toThrow(ProtocolError);
  }
  for (const value of ['\ud800', '\udc00', 'x\ud800A', '\udc00\ud800']) {
    expect(() => unicode({ field: value })).toThrow(ProtocolError);
    expect(() => unicode({ [value]: 1 })).toThrow(ProtocolError);
  }
  expect(() => unicode('😀')).not.toThrow();
  expect(() => parse('{"t":"ping","n":0,"x":' + '['.repeat(10000) + '0' + ']'.repeat(10000) + '}', limits.max_server_frame)).toThrow(ProtocolError);
});
it('bounds encoded frames, including escape expansion', () => {
  const m: ClientMsg = { t: 'submit', client_op_id: id, kind: 'create', file_id: id, path: 'a.md', class: 'A', content: { text: '\x00'.repeat(limits.max_inline_content) } };
  expect(() => encodeClient(m)).toThrow(ProtocolError);
  expect(() => decodeServer(' '.repeat(limits.max_server_frame + 1))).toThrow(ProtocolError);
});
it('retains the trusted replacement boundary', () => {
  const m: ClientMsg = { t: 'submit', client_op_id: id, kind: 'annot_edit', file_id: id, base_version: 1, ops: '{"replace":true}' };
  expect(() => encodeClient(m)).toThrow(ProtocolError);
  expect(() => annotation(m.ops, true, 1000)).not.toThrow();
  for (const ops of ['{"replace":null}', '{"elements":[{"del":{"kind":"strokes","id":"a"}},{"put":{"kind":"strokes","id":"b"},"value":{"id":"b"}}]}', '{"meta":{"put":{"x":1},"del":["x"]}}']) expect(() => annotation(ops, true, 1000)).toThrow(ProtocolError);
});
it('keeps the compile-time code unions synchronized with the generated catalogue', () => {
  const reject: Record<Code, true> = { invalid_submission: true, invalid_op: true, invalid_path: true, too_large: true, rate_limited: true, unknown_file: true, file_exists: true, file_deleted: true, path_taken: true, stale_delete: true, invalid_base: true, read_only_path: true, class_change: true, group_required: true, class_mismatch: true, bulk_delete_guard: true, rebase_required: true, whole_document_required: true, blob_missing: true, internal: true };
  const query: Record<QueryCode, true> = { resync_required: true, invalid_request: true, rate_limited: true, internal: true };
  expect(Object.keys(reject).sort()).toEqual([...codes.reject].sort()); expect(Object.keys(query).sort()).toEqual([...codes.query].sort());
});

it('rejects null at every known server member, including false and zero fields', () => {
  for (const fixture of serverFixtures) {
    const root: unknown = JSON.parse(JSON.stringify(fixture.frame));
    const walk = (v: unknown): void => {
      if (Array.isArray(v)) { for (const child of v as unknown[]) walk(child); }
      else if (v && typeof v === 'object') {
        const obj = v as Record<string, unknown>;
        for (const key of Object.keys(obj)) { const old = obj[key]; obj[key] = null; expect(() => decodeServer(JSON.stringify(root)), `${fixture.name}.${key}`).toThrow(ProtocolError); obj[key] = old; walk(old); }
      }
    };
    walk(root);
  }
});
