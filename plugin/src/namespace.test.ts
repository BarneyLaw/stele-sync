import { Buffer } from 'node:buffer';
import { readFileSync, readdirSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect as assert, it } from 'vitest';
import { parsePath, nfc } from './core/vpath';
import { classify, confirm, expect, defaultClassifyConfig, validClassifyConfig, type Class, type ClassifyConfig, type Demotion } from './core/classify';
import { checkDelete, newDeleteGuard, settle, type Actor } from './core/rules';
const root = resolve(import.meta.dirname, '../../schema');
function fixture<T>(name: string): T { return JSON.parse(readFileSync(resolve(root, name), 'utf8')) as T; }
function path(s: string) { const p = parsePath(s); if (!p.ok) throw new Error(p.reason); return p.path; }
describe('M3 Go contracts', () => {
  it('path reasons and component positions', () => {
    for (const row of fixture<{input_base64: string; reason: string; component: number}[]>('vpath/parse.json')) {
      const got = parsePath(Buffer.from(row.input_base64, 'base64'));
      assert(got.ok ? {reason: 'none', component: -1} : {reason: got.reason, component: got.component}).toEqual({reason: row.reason, component: row.component});
    }
    assert(parsePath('\ud800')).toEqual({ ok: false, reason: 'invalid_utf8', component: -1 });
  });
  it('normalization matches Go, including stream-safe boundaries', () => {
    const data = fixture<{samples: {input: string; nfc: string}[]}>('vpath/normalization.json');
    for (const s of data.samples) assert(nfc(s.input), s.input).toBe(s.nfc);
  });
  it('metadata classification', () => {
    for (const row of fixture<{path: string; size: number; class: Class; config: ClassifyConfig}[]>('classify/expect.json')) {
      assert(validClassifyConfig(row.config)).toBe(true);
      assert(expect(path(row.path), row.size, row.config), row.path).toBe(row.class);
    }
  });
  it('complete-content classification including all M2 limits', () => {
    for (const row of fixture<{name: string; expected: Class; content_base64: string; repeat: number; class: Class; demotion: Demotion; config: ClassifyConfig}[]>('classify/confirm.json')) {
      const unit = Buffer.from(row.content_base64, 'base64'), b = new Uint8Array(unit.length * row.repeat);
      for (let i = 0; i < row.repeat; i++) b.set(unit, i * unit.length);
      assert(confirm(row.expected, b, row.config), `${row.name}: ${unit.toString().slice(0,100)}`).toEqual({ class: row.class, demotion: row.demotion });
    }
  });
  it('all actual FreeDraw samples remain B', () => {
    for (const f of readdirSync(resolve(root, 'annot/samples')).filter(f => f.endsWith('.annot.json'))) {
      assert(classify(path('sample.pdf.annot.json'), readFileSync(resolve(root, 'annot/samples', f)), defaultClassifyConfig()), f).toEqual({ class: 'B', demotion: 'kept' });
    }
  });
  it('bucket history, confirmation and system exemptions', () => {
    type Row = { window_ns: string; max_count: number; min_for_ratio: number; ratio: number; buckets: number; steps: {now_ns: string; actor: Actor; batch: number; live: number; confirmed: boolean; decision: string}[] };
    for (const row of fixture<Row[]>('rules/guard.json')) {
      let g = newDeleteGuard({window: BigInt(row.window_ns), maxCount: row.max_count, minForRatio: row.min_for_ratio, ratio: row.ratio, buckets: row.buckets});
      for (const step of row.steps) {
        const before = structuredClone(g), next = checkDelete(g, BigInt(step.now_ns), step.actor, step.batch, step.live, step.confirmed);
        assert(next.decision).toBe(step.decision); assert(g).toEqual(before);
        if (next.decision === 'needs_confirm' || step.confirmed || step.actor !== 'device') assert(next.guard).toBe(g);
        g = next.guard;
      }
    }
  });
  it('settling and permanent demotion', () => {
    type Row = {existing: Class | ''; confirmed: Class; demotion: Demotion; quiet_ns: string; settlement: string; class: Class};
    for (const row of fixture<Row[]>('rules/settle.json')) assert(settle({existing: row.existing || null, confirmed: row.confirmed, demotion: row.demotion, quiet: BigInt(row.quiet_ns)})).toEqual({settlement: row.settlement, class: row.class});
  });
});
