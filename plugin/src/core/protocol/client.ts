import limits from '../../../../schema/protocol/limits.json';
import { byteLength } from '../vpath';
import { annotation } from './annotation';
import { blob, bool, choice, classes, copyReasons, fail, has, id, integer, object, only, path, str, textOp, unicode } from './validate';
import type { ObjectValue } from './validate';
import type { ClientMsg } from './types';

export type EncodeLimits = { max_frame: number; max_inline_content: number; max_text_op: number; max_annot_op: number };
/** JSON strings retain annotation bytes, including number spelling and escapes. */
export function encodeClient(message: ClientMsg, config: EncodeLimits = limits): string {
  unicode(message); const m = object(message, 'object');
  switch (str(m.t, 'type')) {
    case 'hello': only(m, ['t', 'token']); if (!/^stele_[A-Za-z0-9_-]{43}$/.test(str(m.token, 'token'))) fail('token'); break;
    case 'pull_changes': only(m, ['t', 'req', 'after_seq']); integer(m.req, 'req', 1, 0xffffffff); integer(m.after_seq, 'after_seq'); break;
    case 'list_files': only(m, ['t', 'req', 'after_file_id']); integer(m.req, 'req', 1, 0xffffffff); if (has(m, 'after_file_id')) id(m.after_file_id, 'after_file_id'); break;
    case 'pong': only(m, ['t', 'n']); integer(m.n, 'n', 0, 0xffffffff); break;
    case 'submit': id(m.client_op_id, 'client_op_id'); body(m, config); break;
    default: fail('type');
  }
  const raw = JSON.stringify(message); if (byteLength(raw) > config.max_frame) fail('frame_too_large'); return raw;
}
function body(m: ObjectValue, config: EncodeLimits): void {
  const fields = (names: string[]): void => { only(m, ['t', 'client_op_id', 'kind', ...names]); };
  const base = (): void => { id(m.file_id, 'file_id'); integer(m.base_version, 'base_version', 1); };
  const taken = (): void => { if (has(m, 'on_taken')) choice(m.on_taken, 'on_taken', ['number', 'reject']); };
  switch (str(m.kind, 'kind')) {
    case 'text_edit': fields(['file_id', 'base_version', 'op']); base(); textOp(m.op); if (byteLength(JSON.stringify(m.op)) > config.max_text_op) fail('op'); break;
    case 'annot_edit': fields(['file_id', 'base_version', 'ops']); base(); annotation(m.ops, false, config.max_annot_op); break;
    case 'annot_replace': fields(['file_id', 'base_version', 'content']); base(); content(m.content, 'B', config); break;
    case 'put_blob': fields(['file_id', 'base_version', 'blob']); base(); blob(m.blob, true); break;
    case 'create': fields(['file_id', 'path', 'class', 'content', 'on_taken', 'copy_of']); id(m.file_id, 'file_id'); path(m.path); taken(); content(m.content, choice(m.class, 'class', classes), config);
      if (has(m, 'copy_of')) { const c = object(m.copy_of, 'copy_of'); only(c, ['file_id', 'reason']); id(c.file_id, 'file_id'); choice(c.reason, 'reason', copyReasons); } break;
    case 'rename': fields(['file_id', 'path', 'on_taken']); id(m.file_id, 'file_id'); path(m.path); taken(); break;
    case 'rename_group': {
      fields(['pdf', 'sidecar', 'on_taken']); taken(); const pdf = object(m.pdf, 'pdf'), sidecar = object(m.sidecar, 'sidecar');
      for (const target of [pdf, sidecar]) { only(target, ['file_id', 'path']); id(target.file_id, 'file_id'); path(target.path); }
      if (pdf.file_id === sidecar.file_id) fail('file_id'); break;
    }
    case 'delete': fields(['file_id', 'base_version', 'confirmed']); base(); if (has(m, 'confirmed')) bool(m.confirmed, 'confirmed'); break;
    default: fail('kind');
  }
}
function content(v: unknown, cls: string, config: EncodeLimits): void {
  const c = object(v, 'content'); only(c, ['text', 'sidecar', 'blob']); if (Object.keys(c).length !== 1) fail('content');
  if (has(c, 'blob')) { blob(c.blob, true); return; }
  const key = cls === 'A' ? 'text' : 'sidecar'; if (cls === 'C') fail('content');
  if (byteLength(str(c[key], key)) > config.max_inline_content) fail(key);
}
