import type { Class, Demotion } from './classify';
export type Actor = 'device' | 'bridge' | 'admin';
/** Durations and injected instants are integer nanoseconds, matching Go exactly. */
export interface GuardConfig { window: bigint; maxCount: number; minForRatio: number; ratio: number; buckets: number }
export function defaultGuardConfig(): GuardConfig { return { window: 300000000000n, maxCount: 25, minForRatio: 5, ratio: 0.1, buckets: 30 }; }
interface Bucket { readonly until: bigint; readonly count: number }
export interface DeleteGuard { readonly cfg: Readonly<GuardConfig>; readonly slots: readonly Bucket[]; readonly last: bigint | null }
export function newDeleteGuard(cfg: GuardConfig): DeleteGuard {
  if (cfg.window <= 0n || !Number.isInteger(cfg.buckets) || cfg.buckets < 1 || cfg.buckets > 256 || cfg.window % BigInt(cfg.buckets) !== 0n ||
    !Number.isSafeInteger(cfg.maxCount) || !Number.isSafeInteger(cfg.minForRatio) || cfg.minForRatio < 1 || cfg.maxCount < cfg.minForRatio ||
    !Number.isFinite(cfg.ratio) || cfg.ratio <= 0 || cfg.ratio > 1) throw new Error('invalid_config');
  return { cfg: { ...cfg }, slots: [], last: null };
}
function floorDiv(a: bigint, b: bigint): bigint { const q = a / b; return a % b < 0n ? q - 1n : q; }
export function checkDelete(g: DeleteGuard, now: bigint, actor: Actor, batch: number, live: number, confirmed: boolean): { guard: DeleteGuard; decision: 'apply' | 'needs_confirm' } {
  const reject = () => ({ guard: g, decision: 'needs_confirm' as const });
  const unchanged = () => ({ guard: g, decision: 'apply' as const });
  if (actor === 'bridge' || actor === 'admin' || confirmed) return unchanged();
  if (!Number.isSafeInteger(batch) || batch < 0 || !Number.isSafeInteger(live) || live < 0) return reject();
  if (g.last !== null && now < g.last) now = g.last;
  const cfg = g.cfg, limit = Math.min(cfg.maxCount, Math.max(cfg.minForRatio, Math.floor(cfg.ratio * live)));
  if (batch > limit) return reject(); let remaining = limit - batch;
  for (const slot of g.slots) if (now < slot.until) { if (slot.count > remaining) return reject(); remaining -= slot.count; }
  if (!batch) return unchanged();
  const width = cfg.window / BigInt(cfg.buckets);
  // time.Time.Truncate uses year 1, not the Unix epoch. Preserve that origin even
  // for custom widths which do not divide the epoch offset.
  const origin = 62135596800000000000n;
  const until = floorDiv(now + origin, width) * width - origin + width + cfg.window;
  const slots = g.slots.filter(b => now < b.until && b.until !== until).map(b => ({ ...b }));
  const count = (g.slots.find(b => b.until === until)?.count ?? 0) + batch;
  slots.push({ until, count });
  return { guard: { cfg: { ...cfg }, slots, last: now }, decision: 'apply' };
}
export interface Observation { existing: Class | null; confirmed: Class; demotion: Demotion; quiet: bigint }
export function settle(o: Observation, quiet = 10000000000n): { settlement: 'wait' | 'submit' | 'class_change'; class: Class } {
  if (o.existing === 'C') return { settlement: 'submit', class: 'C' };
  let c = o.confirmed;
  if (o.demotion !== 'kept' || o.existing !== null && o.confirmed === 'C') { c = 'C'; if (o.quiet < quiet) return { settlement: 'wait', class: c }; }
  return { settlement: o.existing !== null && o.existing !== c ? 'class_change' : 'submit', class: c };
}
