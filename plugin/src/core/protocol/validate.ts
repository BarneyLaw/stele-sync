import { byteLength, parsePath } from '../vpath';
import codes from '../../../../schema/protocol/codes.json';

export type ObjectValue = Record<string, unknown>;
/** Contains only a stable field name, never credentials or document bytes. */
export class ProtocolError extends Error {
  constructor(readonly reason: string) { super(`Invalid protocol field: ${reason}`); }
}
export function fail(field: string): never { throw new ProtocolError(field); }
export function object(v: unknown, field: string): ObjectValue {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return fail(field);
  return v as ObjectValue;
}
export function has(v: ObjectValue, k: string): boolean { return Object.prototype.hasOwnProperty.call(v, k); }
export function only(v: ObjectValue, fields: readonly string[]): void { for (const k of Object.keys(v)) if (!fields.includes(k)) fail(k); }
export function str(v: unknown, field: string): string { if (typeof v !== 'string') return fail(field); return v; }
export function integer(v: unknown, field: string, min = 0, max = Number.MAX_SAFE_INTEGER): number {
  if (typeof v !== 'number' || !Number.isSafeInteger(v) || v < min || v > max) return fail(field);
  return v;
}
export function bool(v: unknown, field: string): boolean { if (typeof v !== 'boolean') return fail(field); return v; }
export function array(v: unknown, field: string, max: number): unknown[] { if (!Array.isArray(v) || v.length > max) return fail(field); return v as unknown[]; }
export function choice(v: unknown, field: string, values: readonly string[]): string { const s = str(v, field); if (!values.includes(s)) fail(field); return s; }
export function id(v: unknown, field: string): string { const s = str(v, field); if (!/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(s)) fail(field); return s; }
export function hash(v: unknown, field: string): void { if (!/^[0-9a-f]{64}$/.test(str(v, field))) fail(field); }
export function path(v: unknown): void { if (!parsePath(str(v, 'path')).ok) fail('path'); }
export function blob(v: unknown, strict = false): void { const b = object(v, 'blob'); if (strict) only(b, ['sha256', 'size']); hash(b.sha256, 'sha256'); integer(b.size, 'size'); }
export const classes = ['A', 'B', 'C'];
export const owners = ['user', 'canvas'];
export const copyReasons = codes.reasons.copy_of;
export const originReasons = codes.reasons.origin;
const loneSurrogate = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?:^|[^\uD800-\uDBFF])[\uDC00-\uDFFF]/;
export function unicode(v: unknown, depth = 0): void {
  if (depth > 64) fail('depth');
  if (typeof v === 'string') { if (loneSurrogate.test(v)) fail('unicode'); }
  else if (Array.isArray(v)) for (const item of v as unknown[]) unicode(item, depth + 1);
  else if (v && typeof v === 'object') for (const [k, item] of Object.entries(v)) { unicode(k, depth + 1); unicode(item, depth + 1); }
}
export function parse(frame: string, max: number): ObjectValue {
  if (byteLength(frame) > max) fail('frame_too_large');
  // Bound depth before JSON.parse allocates nested containers.
  let depth = 0, quoted = false, escaped = false;
  for (const c of frame) {
    if (quoted) { if (escaped) escaped = false; else if (c === '\\') escaped = true; else if (c === '"') quoted = false; }
    else if (c === '"') quoted = true;
    else if (c === '[' || c === '{') { if (++depth > 64) fail('depth'); }
    else if (c === ']' || c === '}') depth--;
  }
  let v: unknown; try { v = JSON.parse(frame); } catch { return fail('json'); }
  unicode(v); return object(v, 'object');
}
export function textOp(v: unknown): number {
  const values = array(v, 'op', 65536); let previous = '', base = 0, target = 0;
  for (const item of values) {
    let kind: string;
    if (typeof item === 'string') { if (!item) fail('op'); unicode(item); kind = 'insert'; target += item.length; }
    else { const n = integer(item, 'op', -8388608, 8388608); if (!n) fail('op'); kind = n > 0 ? 'retain' : 'delete'; base += Math.abs(n); if (n > 0) target += n; }
    if (kind === previous || (previous === 'delete' && kind === 'insert')) fail('op'); previous = kind;
    if (base > 8388608 || target > 8388608) fail('op');
  }
  return target;
}
