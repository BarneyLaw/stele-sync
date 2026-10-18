import { byteLength } from '../vpath';
import { array, bool, choice, fail, has, integer, object, only, parse, str } from './validate';

const kinds = ['strokes', 'eraserPaths', 'textItems', 'shapes', 'imageItems'];
const structural = ['appendedPages', 'removedPages'];
/** Validate control fields while returning/forwarding the original string. */
export function annotation(raw: unknown, committed: boolean, maxBytes: number): void {
  const text = str(raw, 'ops'); if (byteLength(text) > maxBytes) fail('ops');
  const root = parse(text, maxBytes); only(root, committed ? ['elements', 'entries', 'meta', 'replace'] : ['elements', 'entries', 'meta']);
  const replace = has(root, 'replace') ? bool(root.replace, 'replace') : false;
  let count = 0, deleting = false, previousKind = -1, previousID = '';
  const seen = new Set<string>();
  for (const rawElement of has(root, 'elements') ? array(root.elements, 'elements', maxBytes) : []) {
    count++; const e = object(rawElement, 'elements'); only(e, ['put', 'del', 'value']);
    const put = has(e, 'put'); if (put === has(e, 'del') || put !== has(e, 'value')) fail('ops');
    const k = object(put ? e.put : e.del, 'key'); only(k, ['kind', 'id']);
    const kind = kinds.indexOf(choice(k.kind, 'kind', kinds)), id = str(k.id, 'id');
    if (!id || byteLength(id) > 128 || seen.has(`${String(kind)}:${id}`)) fail('ops'); seen.add(`${String(kind)}:${id}`);
    if (put) { if (deleting || object(e.value, 'value').id !== id) fail('ops'); }
    else { if (deleting && (kind < previousKind || (kind === previousKind && compareUTF8(id, previousID) <= 0))) fail('ops'); deleting = true; previousKind = kind; previousID = id; }
  }
  let previousFamily = -1, previousPage = 0;
  for (const rawEntry of has(root, 'entries') ? array(root.entries, 'entries', maxBytes) : []) {
    count++; const e = object(rawEntry, 'entries'); only(e, ['put', 'del', 'value']); const put = has(e, 'put');
    if (put === has(e, 'del') || put !== has(e, 'value')) fail('ops');
    const k = object(put ? e.put : e.del, 'key'); only(k, ['family', 'page']);
    const family = ['pdfPageState', 'pdfPageTemplates'].indexOf(choice(k.family, 'family', ['pdfPageState', 'pdfPageTemplates']));
    const page = integer(k.page, 'page', 1, 2147483647);
    if (family < previousFamily || (family === previousFamily && page <= previousPage)) fail('ops'); previousFamily = family; previousPage = page;
    if (put && family === 0) choice(e.value, 'value', ['hidden', 'permanent']);
    if (put && family === 1 && object(e.value, 'value').page !== page) fail('ops');
  }
  if (has(root, 'meta')) {
    const meta = object(root.meta, 'meta'); only(meta, ['put', 'del']);
    const puts = has(meta, 'put') ? object(meta.put, 'put') : {};
    const dels = has(meta, 'del') ? array(meta.del, 'del', maxBytes) : [];
    let previous: string | undefined;
    for (const key of [...Object.keys(puts), ...dels.map(v => str(v, 'del'))]) {
      count++; if ([...kinds, "pdfPageTemplates", "deletedPdfPages", "permanentlyDeletedPdfPages"].includes(key) || (!replace && structural.includes(key))) fail('ops');
      if (has(puts, key) && structural.includes(key)) array(puts[key], key, maxBytes);
    }
    for (const v of dels) { const key = str(v, 'del'); if (has(puts, key) || (previous !== undefined && compareUTF8(previous, key) >= 0)) fail('ops'); previous = key; }
  }
  if (!committed && count > 10000) fail('ops');
}
function compareUTF8(a: string, b: string): number {
  // Go compares UTF-8 bytes; for well-formed strings that is code-point order.
  const aa = Array.from(a), bb = Array.from(b);
  for (let i = 0; i < Math.min(aa.length, bb.length); i++) { const delta = (aa[i]?.codePointAt(0) ?? 0) - (bb[i]?.codePointAt(0) ?? 0); if (delta) return delta; }
  return aa.length - bb.length;
}
