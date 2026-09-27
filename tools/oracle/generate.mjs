// tools/oracle/generate.mjs
//
// Differential-testing oracle for internal/core/textop (M1).
// ot.js computes expected results; Go must agree. ot.js is never a runtime dependency (ADR 001).
//
// Modes:
//   node tools/oracle/generate.mjs                       write schema/textop/{apply,compose,transform}.json
//   node tools/oracle/generate.mjs --stream --seed S --count N [--profile scale]
//                                                        write NDJSON cases to stdout (TestOracleScale)
//
// Invariants this script enforces on everything it emits:
//   - every op is canonical (built through ot.js's builder, which merges and orders components)
//   - no retain/delete/insert boundary splits a surrogate pair (ot.js would happily split one;
//     Go rejects it, so those cases are hand-written in invalid.json, not generated here)
//   - every string is well-formed UTF-16 (no lone surrogates reach the JSON)
//   - output is byte-deterministic for a given seed, so CI can diff regenerated fixtures

import { createRequire } from 'node:module';
import { mkdirSync, writeFileSync, readFileSync, mkdtempSync, renameSync, rmSync, existsSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import { createHash } from 'node:crypto';
import { Readable } from 'node:stream';
import { pipeline } from 'node:stream/promises';
import assert from 'node:assert/strict';

const require = createRequire(import.meta.url);
const { TextOperation } = require('ot');
const here = dirname(fileURLToPath(import.meta.url));
const OT_VERSION = JSON.parse(readFileSync(require.resolve('ot/package.json'), 'utf8')).version;
export const GENERATOR_HASH = createHash('sha256').update(readFileSync(fileURLToPath(import.meta.url), 'utf8').replaceAll('\r\n', '\n')).digest('hex');
export const MAX_RECORD_BYTES = 64 * 1024 * 1024;
export const MAX_COUNT = 10000000;
const TIE = 'a-first';

// ---------- seeded PRNG (Math.random is not seedable) ----------

function mulberry32(seed) {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

class Rng {
  constructor(seed) { this.next = mulberry32(seed); }
  int(lo, hi) { return lo + Math.floor(this.next() * (hi - lo + 1)); } // inclusive
  chance(p) { return this.next() < p; }
  pick(xs) { return xs[this.int(0, xs.length - 1)]; }
}

// ---------- text generation, in code points ----------

// Each pool entry is a sequence of code points. CRLF is one entry of two code points.
const POOLS = {
  ascii: [...'abcdefghijklmnopqrstuvwxyz ABC.,#*-[]'].map(c => [c]),
  newline: [['\n'], ['\r', '\n']],
  cjk: [...'中文字漢語日本語한국어'].map(c => [c]),
  emoji: [...'😀🎉👍🔥🧠📎'].map(c => [c]),      // astral: 2 UTF-16 units each
  combining: [['e', '\u0301'], ['a', '\u0308']], // BMP combining marks: fine to split in UTF-16 terms
  special: [...'"\\<>&\u2028\u2029\ufeff\x00\t\r'].map(c => [c]),
};
const POOL_WEIGHTS = [['ascii', 50], ['newline', 10], ['cjk', 12], ['emoji', 12], ['combining', 6], ['special', 10]];

function randomCodePoints(rng, count) {
  const total = POOL_WEIGHTS.reduce((s, [, w]) => s + w, 0);
  const out = [];
  while (out.length < count) {
    let r = rng.int(1, total);
    let pool;
    for (const [name, w] of POOL_WEIGHTS) { if ((r -= w) <= 0) { pool = POOLS[name]; break; } }
    out.push(...rng.pick(pool));
  }
  out.length = count;
  // Truncation may leave a lone '\r' or 'e' at the end; both are still valid text.
  return out;
}

const units = cps => cps.reduce((n, c) => n + c.length, 0); // UTF-16 length of a code point slice

// ---------- op generation ----------

// Builds a random canonical op over a document given as an array of code points.
// Lengths are chosen in code points and converted to UTF-16 units, so no boundary splits a pair.
function randomOp(rng, docCps, shape) {
  const op = new TextOperation();
  let i = 0;
  const n = docCps.length;
  while (i < n) {
    if (rng.chance(shape.pInsert)) {
      op.insert(randomCodePoints(rng, rng.int(1, shape.maxInsert)).join(''));
    }
    const k = rng.int(1, Math.min(n - i, shape.maxRun));
    const len = units(docCps.slice(i, i + k));
    if (rng.chance(shape.pDelete)) op.delete(len); else op.retain(len);
    i += k;
    assert(op.ops.length <= 65536, 'randomOp component bound');
  }
  if (rng.chance(shape.pInsert)) op.insert(randomCodePoints(rng, rng.int(1, shape.maxInsert)).join(''));
  return op;
}

// Two ops that both insert at the same position: the transform tie case.
function tiePair(rng, docCps) {
  const at = rng.int(0, docCps.length);
  const pre = units(docCps.slice(0, at));
  const post = units(docCps.slice(at));
  const mk = () => {
    const op = new TextOperation().retain(pre).insert(randomCodePoints(rng, rng.int(1, 3)).join(''));
    return rng.chance(0.3) && post > 0 ? op.delete(units(docCps.slice(at, at + 1))).retain(post - units(docCps.slice(at, at + 1))) : op.retain(post);
  };
  return [mk(), mk()];
}

// Two ops whose deletes overlap: the case the guide warns people get wrong.
function overlappingDeletes(rng, docCps) {
  const n = docCps.length;
  if (n < 2) return null;
  const s1 = rng.int(0, n - 1), e1 = rng.int(s1 + 1, n);
  const s2 = rng.int(0, e1 - 1), e2 = rng.int(Math.max(s2 + 1, s1 + 1), n);
  const mk = (s, e) => new TextOperation()
    .retain(units(docCps.slice(0, s))).delete(units(docCps.slice(s, e))).retain(units(docCps.slice(e)));
  return [mk(s1, e1), mk(s2, e2)];
}

const PROFILES = {
  // Small docs dominate: ties and overlaps only happen often when there is little room.
  fixtures: [
    { weight: 45, docMax: 8,   shape: { pInsert: 0.3, pDelete: 0.3, maxRun: 3,  maxInsert: 3 } },
    { weight: 30, docMax: 64,  shape: { pInsert: 0.2, pDelete: 0.2, maxRun: 12, maxInsert: 6 } },
    { weight: 15, docMax: 512, shape: { pInsert: 0.1, pDelete: 0.1, maxRun: 80, maxInsert: 20 } },
    { weight: 10, docMax: 0,   shape: { pInsert: 0.5, pDelete: 0.0, maxRun: 1,  maxInsert: 4 } }, // empty doc
  ],
  // Large and complex coverage is reserved deterministically below. The random
  // mixture is deliberately bounded; its throughput is measured, not assumed.
  scale: [
    { weight: 7000, docMax: 64,     shape: { pInsert: 0.2,  pDelete: 0.2,  maxRun: 12,   maxInsert: 6 } },
    { weight: 2940, docMax: 4096,   shape: { pInsert: 0.1,  pDelete: 0.1,  maxRun: 40,   maxInsert: 20 } },
    { weight: 50,   docMax: 4096,   shape: { pInsert: 0.3,  pDelete: 0.3,  maxRun: 4,    maxInsert: 3 } },  // 500+ component ops
  ],
};

function pickProfile(rng, profiles) {
  let r = rng.int(1, profiles.reduce((s, p) => s + p.weight, 0));
  for (const p of profiles) if ((r -= p.weight) <= 0) return p;
}

function randomDoc(rng, p) {
  return randomCodePoints(rng, p.docMax === 0 ? 0 : rng.int(0, p.docMax));
}

// ---------- case builders ----------

const json = op => op.toJSON();

function applyCase(rng, profiles) {
  const p = pickProfile(rng, profiles);
  const docCps = randomDoc(rng, p);
  const doc = docCps.join('');
  if (rng.chance(0.05)) {
    // Length mismatch: op built for a different document. ot.js throws; Go must return ErrLengthMismatch.
    const other = [...docCps, 'x'];
    const op = randomOp(rng, other, p.shape);
    assert.equal(op.baseLength, doc.length + 1);
    assert.throws(() => op.apply(doc), /base length/);
    inspectOp(op, other.join(''));
    return { doc, op: json(op), error: 'length_mismatch' };
  }
  const op = randomOp(rng, docCps, p.shape);
  return { doc, op: json(op), want: op.apply(doc) };
}

function composeCase(rng, profiles) {
  const p = pickProfile(rng, profiles);
  const docCps = randomDoc(rng, p);
  const doc = docCps.join('');
  const a = randomOp(rng, docCps, p.shape);
  const mid = a.apply(doc);
  const b = randomOp(rng, [...mid], p.shape); // [...str] splits by code point
  const composed = a.compose(b);
  assert.equal(composed.apply(doc), b.apply(mid));
  return { doc, a: json(a), b: json(b), want: json(composed) };
}

function transformCase(rng, profiles) {
  const p = pickProfile(rng, profiles);
  const docCps = randomDoc(rng, p);
  const doc = docCps.join('');
  let pair = null;
  const r = rng.next();
  if (r < 0.15) pair = tiePair(rng, docCps);
  else if (r < 0.30) pair = overlappingDeletes(rng, docCps);
  const [a, b] = pair ?? [randomOp(rng, docCps, p.shape), randomOp(rng, docCps, p.shape)];
  const [ap, bp] = TextOperation.transform(a, b);
  // Self-check: the oracle must itself converge, or it is not an oracle.
  const left = bp.apply(a.apply(doc)), right = ap.apply(b.apply(doc));
  if (left !== right) throw new Error(`ot.js diverged on ${JSON.stringify({ doc, a, b })}`);
  return { doc, a: json(a), b: json(b), want_ap: json(ap), want_bp: json(bp) };
}

const BUILDERS = { apply: applyCase, compose: composeCase, transform: transformCase };

// ---------- guards ----------

function assertWellFormed(value, where) {
  if (typeof value === 'string' && !value.isWellFormed()) throw new Error(`lone surrogate in ${where}`);
  if (Array.isArray(value)) value.forEach((v, i) => assertWellFormed(v, `${where}[${i}]`));
  else if (value && typeof value === 'object') for (const [k, v] of Object.entries(value)) assertWellFormed(v, `${where}.${k}`);
}

// Detect ot.js's tie-break convention instead of assuming it, and fail loudly if it ever changes.
function detectTieBreak() {
  const [ap] = TextOperation.transform(TextOperation.fromJSON(['A']), TextOperation.fromJSON(['B']));
  const s = ap.apply('B');
  assert.equal(s, 'AB', `oracle must use ${TIE} insertion ties`);
  return TIE;
}

// ---------- contract and measured mandatory cases ----------

function inspectOp(op, doc) {
  let pos = 0, previous = '';
  assert.equal(op.baseLength, doc.length);
  for (const c of op.ops) {
    const kind = typeof c === 'string' ? 'i' : c > 0 ? 'r' : 'd';
    assert(typeof c === 'string' ? c.length > 0 && c.isWellFormed() : Number.isSafeInteger(c) && c !== 0);
    assert(kind !== previous && !(previous === 'd' && kind === 'i'), 'noncanonical oracle op');
    previous = kind;
    if (kind === 'i') continue;
    pos += Math.abs(c);
    assert(pos <= doc.length);
    const before = doc.charCodeAt(pos - 1), after = doc.charCodeAt(pos);
    assert(!(before >= 0xd800 && before <= 0xdbff && after >= 0xdc00 && after <= 0xdfff), 'split surrogate');
  }
  assert.equal(pos, doc.length);
}

export function validateCase(c) {
  assertWellFormed(c, c.id);
  const read = value => TextOperation.fromJSON(value);
  if (c.kind === 'apply') {
    const op = read(c.op);
    if (c.error) {
      assert.equal(c.error, 'length_mismatch');
      assert.equal(op.baseLength, c.doc.length + 1);
      inspectOp(op, c.doc + 'x');
      assert.throws(() => op.apply(c.doc), /base length/);
    } else { inspectOp(op, c.doc); assert.equal(op.apply(c.doc), c.want); }
  } else {
    const a = read(c.a), b = read(c.b); inspectOp(a, c.doc);
    const mid = a.apply(c.doc);
    if (c.kind === 'compose') {
      inspectOp(b, mid); const want = read(c.want); inspectOp(want, c.doc);
      assert.deepEqual(a.compose(b).toJSON(), c.want);
      assert.equal(want.apply(c.doc), b.apply(mid));
    } else {
      assert.equal(c.kind, 'transform'); inspectOp(b, c.doc);
      const [ap, bp] = TextOperation.transform(a, b);
      assert.deepEqual(ap.toJSON(), c.want_ap); assert.deepEqual(bp.toJSON(), c.want_bp);
      inspectOp(ap, b.apply(c.doc)); inspectOp(bp, mid);
      assert.equal(ap.apply(b.apply(c.doc)), bp.apply(mid));
    }
  }
}

const CATEGORIES = ['identity', 'unicode', 'tie-cancel', 'overlap', 'delete', 'replace'];
function mandatory(kind, ordinal, profile) {
  let category, doc, a, b;
  if (ordinal < CATEGORIES.length) {
    category = CATEGORIES[ordinal];
    doc = category === 'identity' || category === 'tie-cancel' ? '' : category === 'unicode' ? '\ufeff😀中e\u0301é\r\n\r\n"\\<>&\u2028\u2029\x00�' : 'abcd';
    a = new TextOperation().retain(doc.length);
    if (category === 'unicode') a = new TextOperation().insert(doc).retain(doc.length);
    if (category === 'tie-cancel') a = new TextOperation().insert('😀');
    if (category === 'overlap') a = new TextOperation().retain(1).delete(2).retain(1);
    if (category === 'delete') a = new TextOperation().delete(doc.length);
    if (category === 'replace') a = new TextOperation().insert('X').delete(1).retain(3);
    b = kind === 'compose' ? new TextOperation().retain(a.targetLength) : new TextOperation().retain(doc.length);
    if (category === 'tie-cancel') b = kind === 'compose' ? new TextOperation().delete(2) : new TextOperation().insert('Y');
    if (category === 'overlap' && kind === 'transform') b = new TextOperation().retain(2).delete(2);
  } else if (profile === 'scale' && ordinal < 206) {
    category = ordinal < 106 ? 'large' : 'complex';
    // 4-byte scalar followed by exactly enough ASCII to reach 1 MiB UTF-8.
    const letter = String.fromCharCode(97 + ordinal % 26);
    doc = category === 'large' ? (ordinal % 2 ? '😀' : '🔥') + letter.repeat(1048576 - 4) : letter.repeat(600);
    a = new TextOperation();
    // At least 600 canonical components, even after builder coalescing.
    for (let i = 0; i < 300; i++) a.retain(i === 0 && category === 'large' ? 2 : 1).insert(i % 2 ? '中' : `${letter}${ordinal}`);
    a.retain(doc.length - a.baseLength);
    const baseLength = kind === 'compose' ? a.targetLength : doc.length;
    const offset = 2 + ordinal;
    b = new TextOperation().retain(offset).insert(`B${ordinal}`).delete(1).retain(baseLength - offset - 1);
  } else return null;
  if (kind === 'apply') return { category, doc, op: json(a), want: a.apply(doc) };
  if (kind === 'compose') return { category, doc, a: json(a), b: json(b), want: json(a.compose(b)) };
  const [ap, bp] = TextOperation.transform(a, b);
  return { category, doc, a: json(a), b: json(b), want_ap: json(ap), want_bp: json(bp) };
}

export function integer(value, name, min, max) {
  assert(/^(0|[1-9][0-9]*)$/.test(value), `${name} must be an unsigned decimal integer`);
  const n = Number(value); assert(Number.isSafeInteger(n) && n >= min && n <= max, `${name} outside ${min}..${max}`); return n;
}

export function options(args) {
  const { values } = parseArgs({ args, options: {
    stream: { type: 'boolean', default: false }, kind: { type: 'string' }, seed: { type: 'string' },
    count: { type: 'string' }, profile: { type: 'string' }, out: { type: 'string' },
  }});
  if (!values.stream) {
    for (const key of ['kind', 'seed', 'count', 'profile']) assert(values[key] === undefined, `--${key} requires --stream`);
    return { out: values.out ?? resolve(here, '../../schema/textop') };
  }
  assert(values.out === undefined, '--out cannot be combined with --stream');
  const kind = values.kind ?? 'all', profile = values.profile ?? 'scale';
  assert(kind === 'all' || Object.hasOwn(BUILDERS, kind), 'unknown kind');
  assert(Object.hasOwn(PROFILES, profile), 'unknown profile');
  const seed = integer(values.seed ?? '1', 'seed', 0, 4294967295);
  const minimum = (profile === 'scale' ? 206 : 6) * (kind === 'all' ? 3 : 1);
  const count = integer(values.count ?? '10000000', 'count', minimum, MAX_COUNT);
  return { stream: true, kind, profile, seed, count };
}

function header({kind, profile, seed, count}) {
  return { schema_version: 1, generator_hash: GENERATOR_HASH, ot_version: OT_VERSION, tie_break: TIE,
    kind, profile, seed, count, large_per_kind: profile === 'scale' ? 100 : 0,
    complex_per_kind: profile === 'scale' ? 100 : 0 };
}

function* cases(config) {
  const rng = new Rng(config.seed), kinds = config.kind === 'all' ? Object.keys(BUILDERS) : [config.kind];
  for (let i = 0; i < config.count; i++) {
    const kind = kinds[i % kinds.length], ordinal = Math.floor(i / kinds.length);
    const payload = mandatory(kind, ordinal, config.profile) ?? { category: 'random', ...BUILDERS[kind](rng, PROFILES[config.profile]) };
    const c = { id: `${kind}-${ordinal}`, kind, i, ...payload };
    validateCase(c); yield c;
  }
}

// ---------- output ----------

// One case per line: stable bytes, readable diffs, no giant single-line blobs.
function writeFixture(path, header, cases) {
  const lines = cases.map(c => '    ' + JSON.stringify(c));
  const body = `{\n  "header": ${JSON.stringify(header)},\n  "cases": [\n${lines.join(',\n')}\n  ]\n}\n`;
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, body);
}

const FIXTURE_PLAN = [
  { kind: 'apply', seed: 1001, count: 2000 },
  { kind: 'compose', seed: 2002, count: 2000 },
  { kind: 'transform', seed: 3003, count: 3000 },
];

async function main() {
  const config = options(process.argv.slice(2));
  assert.equal(OT_VERSION, '0.0.15'); detectTieBreak();
  if (config.stream) {
    // pipeline propagates EPIPE/stream errors and applies bounded backpressure.
    function* records() {
      yield JSON.stringify({ header: header(config) }) + '\n';
      for (const c of cases(config)) {
        const line = JSON.stringify(c) + '\n';
        assert(Buffer.byteLength(line) <= MAX_RECORD_BYTES, 'record exceeds harness ceiling');
        yield line;
      }
    }
    await pipeline(Readable.from(records(), { objectMode: false, highWaterMark: 65536 }), process.stdout);
    return;
  }
  const out = resolve(config.out); mkdirSync(dirname(out), { recursive: true });
  const stage = mkdtempSync(join(dirname(out), '.textop-stage-'));
  const installed = [], backups = [];
  try {
    for (const plan of FIXTURE_PLAN) {
      const cfg = { ...plan, profile: 'fixtures' };
      writeFixture(join(stage, `${plan.kind}.json`), header(cfg), [...cases(cfg)]);
      const parsed = JSON.parse(readFileSync(join(stage, `${plan.kind}.json`), 'utf8'));
      assert.equal(parsed.cases.length, plan.count); parsed.cases.forEach(validateCase);
    }
    mkdirSync(out, { recursive: true });
    // Validate the entire set before installation. Roll back completed renames
    // on I/O failure; the hand-written invalid.json is never touched.
    for (const {kind} of FIXTURE_PLAN) {
      const dest = join(out, `${kind}.json`), backup = join(stage, `${kind}.backup`);
      if (existsSync(dest)) { renameSync(dest, backup); backups.push([backup, dest]); }
      renameSync(join(stage, `${kind}.json`), dest); installed.push(dest);
    }
  } catch (err) {
    for (const dest of installed) rmSync(dest);
    for (const [backup, dest] of backups) renameSync(backup, dest);
    throw err;
  } finally { rmSync(stage, { recursive: true, force: true }); }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch(error => { console.error(error.stack); process.exitCode = 1; });
}
