import { asciiLower, base, ext, type VaultPath } from './vpath';
import { defaultAnnotLimits, recognizesSidecar, validAnnotLimits, type AnnotLimits } from './annot-recognition';
export type Class = 'A' | 'B' | 'C';
export type Demotion = 'kept' | 'too_large' | 'invalid_utf8' | 'contains_nul' | 'not_freedraw';
export interface ClassifyConfig { TextExts: string[]; MaxText: number; MaxSidecar: number; Annot: AnnotLimits }
export function defaultClassifyConfig(): ClassifyConfig {
  return { TextExts: ['md', 'txt', 'mdx'], MaxText: 8 << 20, MaxSidecar: 16 << 20, Annot: defaultAnnotLimits() };
}
export function validClassifyConfig(c: ClassifyConfig): boolean {
  return Number.isSafeInteger(c.MaxText) && c.MaxText > 0 && Number.isSafeInteger(c.MaxSidecar) && c.MaxSidecar > 0 &&
    c.Annot.MaxBytes === c.MaxSidecar && validAnnotLimits(c.Annot) &&
    new Set(c.TextExts).size === c.TextExts.length && c.TextExts.every(e => /^[a-z0-9]+$/.test(e));
}
export function isSidecarName(p: VaultPath): boolean { const b = base(p); return b.length > 15 && asciiLower(b).endsWith('.pdf.annot.json'); }
export function expect(p: VaultPath, size: number, cfg: ClassifyConfig): Class {
  if (!Number.isSafeInteger(size) || size < 0) return 'C';
  if (isSidecarName(p)) return size <= cfg.MaxSidecar ? 'B' : 'C';
  if (asciiLower(base(p)).endsWith('.annot.json')) return 'C';
  return cfg.TextExts.includes(ext(p)) && size <= cfg.MaxText ? 'A' : 'C';
}
export function checkText(bytes: Uint8Array, cfg: ClassifyConfig): Demotion {
  if (bytes.length > cfg.MaxText) return 'too_large';
  try { new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes); } catch { return 'invalid_utf8'; }
  return bytes.includes(0) ? 'contains_nul' : 'kept';
}
export function checkSidecar(bytes: Uint8Array, cfg: ClassifyConfig): Demotion {
  if (bytes.length > cfg.MaxSidecar) return 'too_large';
  return recognizesSidecar(bytes, cfg.Annot) ? 'kept' : 'not_freedraw';
}
export function confirm(expected: Class, bytes: Uint8Array, cfg: ClassifyConfig): { class: Class; demotion: Demotion } {
  let demotion: Demotion;
  switch (expected) {
    case 'A': demotion = checkText(bytes, cfg); break;
    case 'B': demotion = checkSidecar(bytes, cfg); break;
    case 'C': return { class: 'C', demotion: 'kept' };
    default: { const exhaustive: never = expected; throw new Error(`Unknown class ${String(exhaustive)}`); }
  }
  return { class: demotion === 'kept' ? expected : 'C', demotion };
}
export function classify(p: VaultPath, bytes: Uint8Array, cfg: ClassifyConfig): { class: Class; demotion: Demotion } {
  return confirm(expect(p, bytes.length, cfg), bytes, cfg);
}
