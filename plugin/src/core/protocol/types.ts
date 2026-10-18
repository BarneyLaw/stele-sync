/** Wire values for stele.v1; all counters are safe integers after validation. */
export type ID = string;
export type Class = 'A' | 'B' | 'C';
export type Owner = 'user' | 'canvas';
export type CopyReason = 'rebase' | 'deleted' | 'class_change' | 'first_sync';
export type OriginReason = CopyReason | 'stale_put' | 'stale_replace';
export type BlobRef = { sha256: string; size: number };
export type Content = { text: string } | { sidecar: string } | { blob: BlobRef };
export type Target = { file_id: ID; path: string };
export type TextOp = readonly (number | string)[];
type Base = { file_id: ID; base_version: number };
type OnTaken = { on_taken?: 'number' | 'reject' };
export type Body =
  | ({ kind: 'text_edit'; op: TextOp } & Base)
  | ({ kind: 'annot_edit'; ops: string } & Base)
  | ({ kind: 'annot_replace'; content: { sidecar: string } | { blob: BlobRef } } & Base)
  | ({ kind: 'put_blob'; blob: BlobRef } & Base)
  | ({ kind: 'create'; file_id: ID; path: string; class: Class; content: Content; copy_of?: { file_id: ID; reason: CopyReason } } & OnTaken)
  | ({ kind: 'rename' } & Target & OnTaken)
  | ({ kind: 'rename_group'; pdf: Target; sidecar: Target } & OnTaken)
  | ({ kind: 'delete'; confirmed?: boolean } & Base);
export type ClientMsg =
  | { t: 'hello'; token: string }
  | { t: 'pull_changes'; req: number; after_seq: number }
  | { t: 'list_files'; req: number; after_file_id?: ID }
  | ({ t: 'submit'; client_op_id: ID } & Body)
  | { t: 'pong'; n: number };
export type Payload = { op: TextOp; len: number } | { ops: string; hash: string } | { blob: BlobRef } |
  { omitted: true; len: number } | { omitted: true; hash: string };
type Identity = { file_id: ID; version: number };
export type Member = Identity & (
  | ({ kind: 'create'; path: string; class: Class; owner: Owner; origin?: { copy_of: ID; reason: OriginReason } } & Payload)
  | ({ kind: 'text_edit' } & ({ op: TextOp; len: number } | { omitted: true; len: number }))
  | ({ kind: 'annot_edit' } & ({ ops: string; hash: string } | { omitted: true; hash: string }))
  | { kind: 'put_blob'; blob: BlobRef }
  | { kind: 'rename'; path: string }
  | { kind: 'delete' });
export type Change = { seq: number; at: string; actor: 'device' | 'bridge' | 'admin'; device_id: ID; client_op_id: ID; members: Member[] };
export type FileInfo = Identity & { path: string; owner: Owner; size: number } & (
  { class: 'A'; len: number } | { class: 'B'; hash: string } | { class: 'C'; blob: BlobRef });
export type Advertised = {
  max_frame: number; max_server_frame: number; split_bytes: number; page_entries: number; inflight_per_device: number;
  rate: { per_sec: number; burst: number };
  classify: { text_exts: string[]; max_text: number; max_sidecar: number };
  guard: { window_ms: number; max_count: number; min_for_ratio: number; ratio: number; buckets: number };
};
export type Code = 'invalid_submission' | 'invalid_op' | 'invalid_path' | 'too_large' | 'rate_limited' |
  'unknown_file' | 'file_exists' | 'file_deleted' | 'path_taken' | 'stale_delete' | 'invalid_base' | 'read_only_path' |
  'class_change' | 'group_required' | 'class_mismatch' | 'bulk_delete_guard' | 'rebase_required' | 'whole_document_required' |
  'blob_missing' | 'internal';
export type QueryCode = 'resync_required' | 'invalid_request' | 'rate_limited' | 'internal';
export type Detail = { field?: string; reason?: string; component?: number; class?: Class; demotion?: 'kept' | 'too_large' | 'invalid_utf8' | 'contains_nul' | 'not_freedraw'; limit?: number; scope?: 'rate' | 'file' | 'device' | 'mailbox'; retry_after_ms?: number };
export type ServerMsg =
  | { t: 'welcome'; device_id: ID; head_seq: number; limits: Advertised }
  | ({ t: 'change' } & Change)
  | { t: 'changes'; req: number; entries: Change[]; more: boolean }
  | { t: 'files'; req: number; head_seq: number; files: FileInfo[]; more: boolean }
  | { t: 'reject'; client_op_id: ID; code: Code; detail?: Detail }
  | { t: 'error'; req: number; code: QueryCode; detail?: Detail }
  | { t: 'ping'; n: number };
