import codes from '../../../../schema/protocol/codes.json';
import limits from '../../../../schema/protocol/limits.json';
import { annotation } from './annotation';
import { array, blob, bool, choice, classes, fail, has, hash, id, integer, object, originReasons, owners, parse, path, str, textOp } from './validate';
import type { ObjectValue } from './validate';
import type { ServerMsg } from './types';

export type DecodeLimits = { max_server_frame: number; page_entries: number };
/** Server extensions are ignored; every known member is validated before use. */
export function decodeServer(frame: string, config: DecodeLimits = limits): ServerMsg {
  const m = parse(frame, config.max_server_frame);
  switch (str(m.t, 'type')) {
    case 'welcome': id(m.device_id, 'device_id'); integer(m.head_seq, 'head_seq'); advertised(m.limits); break;
    case 'change': change(m); break;
    case 'changes': {
      integer(m.req, 'req', 1, 0xffffffff); const more = bool(m.more, 'more'); const entries = array(m.entries, 'entries', config.page_entries);
      if (more && !entries.length) fail('entries'); let seq: number | undefined;
      for (const entry of entries) { const ch = object(entry, 'entries'); change(ch); const next = integer(ch.seq, 'seq', 1); if (seq !== undefined && next !== seq + 1) fail('entries'); seq = next; }
      break;
    }
    case 'files': {
      integer(m.req, 'req', 1, 0xffffffff); integer(m.head_seq, 'head_seq'); const more = bool(m.more, 'more'); const files = array(m.files, 'files', config.page_entries);
      if (more && !files.length) fail('files'); let previous = '';
      for (const raw of files) { const f = object(raw, 'files'); const key = id(f.file_id, 'file_id'); if (key <= previous) fail('files'); previous = key; file(f); }
      break;
    }
    case 'reject': id(m.client_op_id, 'client_op_id'); choice(m.code, 'code', codes.reject); if (has(m, 'detail')) detail(m.detail); break;
    case 'error': integer(m.req, 'req', 1, 0xffffffff); choice(m.code, 'code', codes.query); if (has(m, 'detail')) detail(m.detail); break;
    case 'ping': integer(m.n, 'n', 0, 0xffffffff); break;
    default: fail('type');
  }
  return m as ServerMsg;
}
function change(c: ObjectValue): void {
  integer(c.seq, 'seq', 1); id(c.device_id, 'device_id'); id(c.client_op_id, 'client_op_id'); choice(c.actor, 'actor', ['device', 'bridge', 'admin']);
  const at = str(c.at, 'at'); if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$/.test(at)) fail('at');
  const date = new Date(at); if (!Number.isFinite(date.getTime()) || date.toISOString() !== at) fail('at');
  const members = array(c.members, 'members', 2); if (!members.length) fail('members');
  for (const m of members) member(object(m, 'members'));
  if (members.length === 2) { const a = object(members[0], 'members'), b = object(members[1], 'members'); if (a.kind !== 'rename' || b.kind !== 'rename' || a.file_id === b.file_id) fail('members'); }
}
function member(m: ObjectValue): void {
  id(m.file_id, 'file_id'); integer(m.version, 'version', 1); const kind = str(m.kind, 'kind'); let cls = '';
  switch (kind) {
    case 'create': path(m.path); cls = choice(m.class, 'class', classes); choice(m.owner, 'owner', owners);
      if (has(m, 'origin')) { const o = object(m.origin, 'origin'); id(o.copy_of, 'copy_of'); choice(o.reason, 'reason', originReasons); } break;
    case 'text_edit': cls = 'A'; break;
    case 'annot_edit': cls = 'B'; break;
    case 'put_blob': cls = 'C'; break;
    case 'rename': path(m.path); break;
    case 'delete': break;
    default: fail('kind');
  }
  if (kind !== 'create') { for (const key of ['class', 'owner', 'origin']) if (has(m, key)) fail(key); if (kind !== 'rename' && has(m, 'path')) fail('path'); }
  const count = ['op', 'ops', 'blob', 'omitted'].filter(k => has(m, k)).length;
  if (!cls) { if (count || has(m, 'len') || has(m, 'hash')) fail('payload'); return; }
  if (count !== 1) fail('payload');
  if (cls === 'A') { integer(m.len, 'len'); if (has(m, 'hash')) fail('hash'); }
  if (cls === 'B') { hash(m.hash, 'hash'); if (has(m, 'len')) fail('len'); }
  if (cls === 'C' && (has(m, 'len') || has(m, 'hash'))) fail('payload');
  if (has(m, 'omitted')) { if (m.omitted !== true || cls === 'C') fail('omitted'); return; }
  if (cls === 'A' && textOp(m.op) !== m.len) fail('len');
  if (cls === 'B') annotation(m.ops, true, 64 << 20);
  if (cls === 'C') blob(m.blob);
}
function file(f: ObjectValue): void {
  path(f.path); integer(f.version, 'version', 1); integer(f.size, 'size'); choice(f.owner, 'owner', owners); const cls = choice(f.class, 'class', classes);
  if (['len', 'hash', 'blob'].filter(k => has(f, k)).length !== 1) fail('payload');
  if (cls === 'A') integer(f.len, 'len');
  if (cls === 'B') hash(f.hash, 'hash');
  if (cls === 'C') { blob(f.blob); if (object(f.blob, 'blob').size !== f.size) fail('size'); }
}
function detail(v: unknown): void {
  const d = object(v, 'detail'); for (const k of ['field', 'reason']) if (has(d, k)) str(d[k], k);
  if (has(d, 'component')) integer(d.component, 'component', -1);
  if (has(d, 'class')) choice(d.class, 'class', classes);
  if (has(d, 'demotion')) choice(d.demotion, 'demotion', ['kept', 'too_large', 'invalid_utf8', 'contains_nul', 'not_freedraw']);
  if (has(d, 'limit')) integer(d.limit, 'limit');
  if (has(d, 'retry_after_ms')) integer(d.retry_after_ms, 'retry_after_ms');
  if (has(d, 'scope')) choice(d.scope, 'scope', ['rate', 'file', 'device', 'mailbox']);
}
function advertised(v: unknown): void {
  const a = object(v, 'limits'); for (const k of ['max_frame', 'max_server_frame', 'split_bytes', 'page_entries', 'inflight_per_device']) integer(a[k], k, 1);
  const rate = object(a.rate, 'rate'); integer(rate.per_sec, 'per_sec', 1); integer(rate.burst, 'burst', 1);
  const c = object(a.classify, 'classify'); integer(c.max_text, 'max_text', 1); integer(c.max_sidecar, 'max_sidecar', 1);
  const exts = array(c.text_exts, 'text_exts', 1024), seen = new Set<string>();
  for (const ext of exts) { const s = str(ext, 'text_exts'); if (!/^[a-z0-9]+$/.test(s) || seen.has(s)) fail('text_exts'); seen.add(s); }
  const g = object(a.guard, 'guard'); for (const k of ['window_ms', 'max_count', 'min_for_ratio', 'buckets']) integer(g[k], k, 1);
  if (typeof g.ratio !== 'number' || !Number.isFinite(g.ratio) || g.ratio <= 0 || g.ratio > 1) fail('ratio');
  if (integer(a.split_bytes, 'split_bytes') > integer(a.max_frame, 'max_frame') || integer(a.split_bytes, 'split_bytes') > Math.floor(integer(a.max_server_frame, 'max_server_frame') / 2) || integer(g.min_for_ratio, 'min_for_ratio') > integer(g.max_count, 'max_count') || integer(g.buckets, 'buckets') > 10000 || integer(g.window_ms, 'window_ms') < integer(g.buckets, 'buckets')) fail('limits');
}
