import { fail, integer } from './validate';
/** Pure UUIDv7 generation: callers inject time and ten cryptographic random bytes. */
export function uuidv7(unixMilli: number, entropy: Uint8Array): string {
  integer(unixMilli, 'unix_milli', 0, 2 ** 48 - 1); if (entropy.length !== 10) fail('entropy');
  const bytes = new Uint8Array(16); bytes.set(entropy, 6);
  for (let i = 5; i >= 0; i--) { bytes[i] = unixMilli % 256; unixMilli = Math.floor(unixMilli / 256); }
  bytes[6] = ((bytes[6] ?? 0) & 15) | 0x70; bytes[8] = ((bytes[8] ?? 0) & 63) | 0x80;
  const text = Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
  return `${text.slice(0, 8)}-${text.slice(8, 12)}-${text.slice(12, 16)}-${text.slice(16, 20)}-${text.slice(20)}`;
}
