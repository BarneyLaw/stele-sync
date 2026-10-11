/** Recognition-only port of M2 Parse, including raw JSON and materialized budgets.
 * This does not implement annotation operations or authorize a whole-document edit.
 */
import { byteLength, wellFormed } from './vpath';
export interface AnnotLimits {
  MaxBytes: number; MaxElements: number; MaxIDBytes: number;
  MaxOpBytes: number; MaxStateBytes: number; MaxOps: number; MaxTombstones: number; MaxDepth: number; MaxNodes: number;
}
export function defaultAnnotLimits(): AnnotLimits {
  return { MaxBytes: 16 << 20, MaxElements: 200000, MaxIDBytes: 128, MaxOpBytes: 1 << 20, MaxStateBytes: 64 << 20,
    MaxOps: 10000, MaxTombstones: 200000, MaxDepth: 64, MaxNodes: 2000000 };
}
export function validAnnotLimits(l: AnnotLimits): boolean {
  return Object.values(l).every(n => Number.isSafeInteger(n) && n >= 0) && l.MaxBytes > 0 && l.MaxElements > 0 && l.MaxIDBytes > 0;
}
type Node = { kind: 'scalar'; raw: string } | { kind: 'array'; items: Node[] } | { kind: 'object'; members: Map<string, Node>; names: Map<string, string> };
function invalid(): never { throw new Error('not_freedraw'); }
function object(n: Node | undefined): Map<string, Node> { if (n?.kind !== 'object') return invalid(); return n.members; }
function array(n: Node | undefined): Node[] { if (n?.kind !== 'array') return invalid(); return n.items; }
function rawString(n: Node | undefined): string {
  if (n?.kind !== 'scalar' || !n.raw.startsWith('"')) return invalid();
  const value: unknown = JSON.parse(n.raw); if (typeof value !== 'string') return invalid(); return value;
}
/** A bounded recursive parser avoids JSON.parse's duplicate-key and surrogate repair. */
function parseJSON(s: string, limits: AnnotLimits): Node {
  let offset = 0, nodes = 0;
  const whitespace = () => { while (/[\t\n\r ]/.test(s.charAt(offset)) && offset < s.length) offset++; };
  const stringToken = (): string => {
    const start = offset++; let escaped = false;
    while (offset < s.length) {
      const c = s.charAt(offset++);
      if (c === '"' && !escaped) {
        const raw = s.slice(start, offset), decoded: unknown = JSON.parse(raw);
        if (typeof decoded !== 'string' || !wellFormed(decoded)) return invalid(); return raw;
      }
      if (c === '\\' && !escaped) escaped = true; else escaped = false;
    }
    return invalid();
  };
  const value = (depth: number): Node => {
    if (depth > (limits.MaxDepth || 64) || ++nodes > (limits.MaxNodes || 2000000)) return invalid();
    whitespace(); const c = s.charAt(offset);
    if (c === '"') return { kind: 'scalar', raw: stringToken() };
    if (c === '{' || c === '[') {
      const members = new Map<string, Node>(), names = new Map<string, string>(), items: Node[] = []; const end = c === '{' ? '}' : ']'; offset++; whitespace();
      if (s.charAt(offset) !== end) for (;;) {
        if (c === '{') {
          if (s.charAt(offset) !== '"') return invalid();
          const keyRaw = stringToken(), key: unknown = JSON.parse(keyRaw);
          if (typeof key !== 'string' || members.has(key)) return invalid();
          whitespace(); if (s.charAt(offset++) !== ':') return invalid(); members.set(key, value(depth + 1)); names.set(key, keyRaw);
        } else items.push(value(depth + 1));
        whitespace(); if (s.charAt(offset) === end) break;
        if (s.charAt(offset++) !== ',') return invalid(); whitespace();
      }
      offset++; return c === '{' ? { kind: 'object', members, names } : { kind: 'array', items };
    }
    const match = /^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/.exec(s.slice(offset));
    if (!match) return invalid(); offset += match[0].length; return { kind: 'scalar', raw: match[0] };
  };
  const root = value(1); whitespace(); if (offset !== s.length) return invalid(); return root;
}
function page(n: Node | undefined): number {
  if (n?.kind !== 'scalar' || !/^[1-9]\d*$/.test(n.raw)) return invalid();
  const p = Number(n.raw); if (p > 2147483647) return invalid(); return p;
}
const collections = ['strokes', 'eraserPaths', 'textItems', 'shapes', 'imageItems'] as const;
/** Count Go json.Indent output without allocating an expanded string. */
function materializedSize(root: Node, max: number): number {
  let size = 0;
  const add = (n: number) => { size += n; if (size > max) invalid(); };
  const visit = (n: Node, depth: number): void => {
    if (n.kind === 'scalar') { add(byteLength(n.raw)); return; }
    const count = n.kind === 'array' ? n.items.length : n.members.size;
    add(2); if (!count) return;
    add(count * (1 + (depth + 1) * 2) + count - 1 + 1 + depth * 2);
    if (n.kind === 'array') for (const item of n.items) visit(item, depth + 1);
    else for (const [key, val] of n.members) {
      // Go re-encodes top-level names but preserves escaped names inside raw values.
      const encoded = depth === 0 ? JSON.stringify(key).replace(/\u2028/g, '\\u2028').replace(/\u2029/g, '\\u2029') : (n.names.get(key) ?? JSON.stringify(key));
      add(byteLength(encoded) + 2);
      visit(val, depth + 1);
    }
  };
  visit(root, 0); return size;
}
export function recognizesSidecar(bytes: Uint8Array, limits: AnnotLimits): boolean {
  if (!validAnnotLimits(limits) || bytes.length > limits.MaxBytes) return false;
  try {
    const text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
    const root = parseJSON(text, limits), fields = object(root);
    if (!fields.has('strokes')) return false;
    let count = 0;
    for (const name of collections) {
      const values = fields.has(name) ? array(fields.get(name)) : [];
      const ids = new Set<string>();
      for (const val of values) {
        const id = rawString(object(val).get('id'));
        if (!id || byteLength(id) > limits.MaxIDBytes || ids.has(id) || ++count > limits.MaxElements) return false;
        ids.add(id);
      }
      fields.set(name, { kind: 'array', items: values });
    }
    const pages = new Set<number>(); let templates: Node[] = [];
    const hidden = new Map<number, Node>(), permanent = new Map<number, Node>();
    for (const name of ['deletedPdfPages', 'permanentlyDeletedPdfPages', 'pdfPageTemplates']) {
      const values = fields.has(name) ? array(fields.get(name)) : [], seen = new Set<number>();
      for (const val of values) {
        const p = page(name === 'pdfPageTemplates' ? object(val).get('page') : val);
        if (seen.has(p)) return false; seen.add(p);
        if (name === 'pdfPageTemplates') { count++; } else if (!pages.has(p)) { pages.add(p); count++; }
        if (count > limits.MaxElements) return false;
        if (name === 'deletedPdfPages') hidden.set(p, { kind: 'scalar', raw: String(p) });
        if (name === 'permanentlyDeletedPdfPages') permanent.set(p, { kind: 'scalar', raw: String(p) });
      }
      if (name === 'pdfPageTemplates') templates = values;
    }
    for (const p of permanent.keys()) hidden.delete(p);
    fields.set('deletedPdfPages', { kind: 'array', items: [...hidden.values()] });
    fields.set('permanentlyDeletedPdfPages', { kind: 'array', items: [...permanent.values()] });
    fields.set('pdfPageTemplates', { kind: 'array', items: templates });
    for (const name of ['appendedPages', 'removedPages']) if (fields.has(name)) array(fields.get(name));
    return materializedSize(root, limits.MaxBytes) <= limits.MaxBytes;
  } catch { return false; }
}
