// Byte-level file handling for the PuTTY import dialog.
//
// Both helpers exist because `File.text()` always decodes as UTF-8, which is
// the wrong assumption for the two files this dialog accepts:
//
//   .reg  — `regedit /e` writes UTF-16LE with a BOM. Decoded as UTF-8 it
//           becomes NUL-interleaved text that matches no directive, so the
//           import silently found zero sessions.
//   .ppk  — a key file is bytes, not text. Round-tripping it through a string
//           and btoa() throws on any character above U+00FF, so a key with a
//           non-ASCII Comment: line crashed the file picker.

/** Byte-order marks, longest first so UTF-8's 3-byte mark is tested before pairs. */
const BOMS: { bytes: number[]; encoding: string }[] = [
  { bytes: [0xef, 0xbb, 0xbf], encoding: 'utf-8' },
  { bytes: [0xff, 0xfe], encoding: 'utf-16le' },
  { bytes: [0xfe, 0xff], encoding: 'utf-16be' }
];

/** How many leading bytes the BOM-less heuristic inspects. */
const SNIFF_LIMIT = 1024;

/**
 * Decodes a text file, honouring its byte-order mark and falling back to a
 * UTF-16 heuristic when there is none.
 *
 * The BOM is stripped: `Windows Registry Editor Version 5.00` has to be the
 * literal first line for the file to look like a registry export, and a
 * leading U+FEFF would also survive into the first parsed value.
 */
export function decodeTextFile(bytes: Uint8Array): string {
  for (const bom of BOMS) {
    if (startsWith(bytes, bom.bytes)) {
      return decode(bom.encoding, bytes.subarray(bom.bytes.length));
    }
  }
  return decode(sniffEncoding(bytes), bytes);
}

/**
 * Guesses an encoding for content with no BOM.
 *
 * UTF-16 text that is mostly ASCII — which every registry export is — carries
 * one NUL per character, and their alignment gives the byte order away: the
 * little-endian form puts them at odd offsets, the big-endian form at even
 * ones. Plain UTF-8 text has no NUL bytes at all, so any meaningful count is
 * already proof the file is not UTF-8.
 */
function sniffEncoding(bytes: Uint8Array): string {
  const limit = Math.min(bytes.length, SNIFF_LIMIT);
  if (limit < 2) return 'utf-8';

  let atEven = 0;
  let atOdd = 0;
  for (let i = 0; i < limit; i++) {
    if (bytes[i] !== 0) continue;
    if (i % 2 === 0) atEven++;
    else atOdd++;
  }

  // A quarter of the sample being NUL is far past anything real UTF-8 text
  // produces, and well under the ~50% that pure-ASCII UTF-16 reaches.
  const threshold = limit / 8;
  if (atOdd > threshold && atOdd > atEven * 4) return 'utf-16le';
  if (atEven > threshold && atEven > atOdd * 4) return 'utf-16be';
  return 'utf-8';
}

function startsWith(bytes: Uint8Array, prefix: number[]): boolean {
  if (bytes.length < prefix.length) return false;
  return prefix.every((b, i) => bytes[i] === b);
}

function decode(encoding: string, bytes: Uint8Array): string {
  try {
    return new TextDecoder(encoding).decode(bytes);
  } catch {
    // An engine without this encoding is better served by mojibake than by a
    // thrown error that loses the file entirely.
    return new TextDecoder('utf-8').decode(bytes);
  }
}

/**
 * Base64-encodes raw bytes for transport over the Wails bridge.
 *
 * Chunked because `String.fromCharCode(...bytes)` spreads every byte into an
 * argument list and blows the call-stack limit on files of any size.
 */
export function bytesToBase64(bytes: Uint8Array): string {
  const CHUNK = 0x8000;
  let binary = '';
  for (let i = 0; i < bytes.length; i += CHUNK) {
    binary += String.fromCharCode(...bytes.subarray(i, i + CHUNK));
  }
  return btoa(binary);
}
