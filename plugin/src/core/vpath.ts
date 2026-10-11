import normalization from '../../../schema/vpath/normalization.json';

/** Device-path legality only. Filesystem keys are computed exclusively by Go. */
declare const pathBrand: unique symbol;
export type VaultPath = string & { readonly [pathBrand]: true };
export type PathReason = 'none' | 'invalid_utf8' | 'not_nfc' | 'empty' | 'absolute' | 'backslash' |
  'empty_component' | 'dot_component' | 'traversal' | 'illegal_char' | 'control_char' |
  'leading_space' | 'trailing_dot_or_space' | 'reserved_name' | 'component_too_long' | 'path_too_long';
export type PathResult = { readonly ok: true; readonly path: VaultPath } |
  { readonly ok: false; readonly reason: Exclude<PathReason, 'none'>; readonly component: number };
export const deviceMaxComponent = 200;
export const deviceMaxTotal = 512;
export function byteLength(s: string): number { return new TextEncoder().encode(s).length; }
export function wellFormed(s: string): boolean {
  for (const rune of s) { const n = rune.codePointAt(0) ?? 0; if (n >= 0xd800 && n <= 0xdfff) return false; }
  return true;
}
export function asciiLower(s: string): string { return s.replace(/[A-Z]/g, c => c.toLowerCase()); }
/** Normalize with the server's pinned Unicode assignment boundary. Unicode
 * normalization stability covers assigned characters; future assignments must
 * remain inert starters as they were in the server's table version.
 */
export function nfc(s: string): string {
  const lookup = (r: number, ranges: readonly (readonly number[])[]): readonly number[] | undefined => {
    let low = 0, high = ranges.length;
    while (low < high) {
      const middle = (low + high) >>> 1, range = ranges[middle];
      if (!range) return undefined;
      const first = range[0] ?? 0, last = range[1] ?? 0;
      if (r < first) high = middle;
      else if (r > last) low = middle + 1;
      else return range;
    }
    return undefined;
  };
  let result = '', run = '', nonStarters = 0;
  for (const r of s) {
    const code = r.codePointAt(0) ?? 0;
    const stream = lookup(code, normalization.stream_ranges), lead = stream?.[2] ?? 0, trail = stream?.[3] ?? 0;
    // x/text uses stream-safe normalization, including backward-combining
    // starters such as Jamo. Counts come from its pinned NFKD properties.
    if (nonStarters + lead > 30) { run += '\u034f'; nonStarters = 0; }
    nonStarters = lead ? nonStarters + lead : trail;
    if (lookup(code, normalization.assigned_ranges)) run += r;
    else { result += run.normalize('NFC') + r; run = ''; }
  }
  return result + run.normalize('NFC');
}
export function parsePath(input: string | Uint8Array): PathResult {
  const fail = (reason: Exclude<PathReason, 'none'>, component = -1): PathResult => ({ ok: false, reason, component });
  let s: string;
  try { s = typeof input === 'string' ? input : new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(input); }
  catch { return fail('invalid_utf8'); }
  if (!wellFormed(s)) return fail('invalid_utf8');
  if (nfc(s) !== s) return fail('not_nfc');
  if (!s) return fail('empty');
  if (s.startsWith('/')) return fail('absolute');
  if (s.includes('\\')) return fail('backslash');
  const parts = s.split('/');
  for (const [i, c] of parts.entries()) {
    if (!c) return fail('empty_component', i);
    if (c === '.') return fail('dot_component', i);
    if (c === '..') return fail('traversal', i);
  }
  for (const [i, c] of parts.entries()) {
    if (/[<>:"|?*]/u.test(c)) return fail('illegal_char', i);
    // Unicode Cc is exactly C0, DEL and C1; format characters are not controls.
    if (Array.from(c).some(r => { const n = r.codePointAt(0) ?? 0; return n < 32 || (n >= 127 && n <= 159); })) return fail('control_char', i);
    if (c.startsWith(' ')) return fail('leading_space', i);
    if (/[. ]$/u.test(c)) return fail('trailing_dot_or_space', i);
    const stem = (c.indexOf('.') > 0 ? c.slice(0, c.indexOf('.')) : c).replace(/ +$/u, '');
    if (/^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9¹²³]|LPT[1-9¹²³])$/u.test(asciiLower(stem).toUpperCase())) return fail('reserved_name', i);
  }
  for (const [i, c] of parts.entries()) if (byteLength(c) > deviceMaxComponent) return fail('component_too_long', i);
  if (byteLength(s) > deviceMaxTotal) return fail('path_too_long');
  return { ok: true, path: s as VaultPath };
}
export function base(p: VaultPath): string { return p.slice(p.lastIndexOf('/') + 1); }
export function ext(p: VaultPath): string { const b = base(p), dot = b.lastIndexOf('.'); return dot > 0 ? Array.from(b.slice(dot + 1), r => r === 'İ' ? 'i' : r.toLowerCase()).join('') : ''; }
