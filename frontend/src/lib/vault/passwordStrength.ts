// Offline master-password strength evaluation.
//
// Deliberately dependency-free and import-free: this is security-adjacent code
// that runs on every keystroke of the create-vault screen, so it stays a pure,
// synchronous function with no I/O, no state and nothing to race against. That
// also keeps it outside every rule in src/architecture.test.ts.
//
// The estimate is an entropy approximation with pattern penalties, not a
// guessability oracle. It is advisory: the only hard gate is the length floor
// below. A password can score "strong" here and still be weak against an
// attacker with a good wordlist, which is why the create screen warns rather
// than pretends to certify.

/**
 * Hard floor for a new master password, in code points.
 *
 * Mirrors domain.MinMasterPasswordLength in internal/domain/vault_policy.go,
 * which is the authoritative check. There is no Go-to-TypeScript constant
 * generation in this repository, so the two must be changed together.
 */
export const MIN_MASTER_PASSWORD_LENGTH = 8;

/** Length at which the checklist stops nagging. Advisory only, never blocking. */
export const RECOMMENDED_MASTER_PASSWORD_LENGTH = 12;

/** Entropy that fills the meter completely. */
const FULL_BAR_BITS = 80;

/** Band edges, in estimated bits. */
const MEDIUM_BITS = 36;
const STRONG_BITS = 60;

/**
 * Entropy multiplier by number of character classes, indexed 0..4.
 *
 * A narrow alphabet is worth less per character than the pool size alone
 * suggests, because that is precisely the search space an attacker restricts
 * themselves to first.
 */
const CLASS_FACTOR = [0, 0.55, 0.8, 0.95, 1];

export type StrengthLabel = 'weak' | 'medium' | 'strong';

export interface StrengthResult {
  /** Estimated entropy after penalties, in bits. */
  entropyBits: number;
  /** 0..100, for the meter fill. */
  score: number;
  label: StrengthLabel;
  /** Human-readable reasons the estimate was reduced. */
  warnings: string[];
}

export interface PasswordChecklist {
  /** The only blocking requirement. */
  minLength: boolean;
  recommendedLength: boolean;
  hasLower: boolean;
  hasUpper: boolean;
  hasDigit: boolean;
  hasSymbol: boolean;
}

// The published head of the most-reused password lists (RockYou / NCSC top 100),
// plus a handful of entries specific to this app that a user might reach for.
// Inline and lowercase on purpose: no fetch, no data file, O(1) lookup.
const COMMON_PASSWORDS: ReadonlySet<string> = new Set([
  '123456', '123456789', '12345678', '1234567', '12345', '1234567890', '1234',
  '111111', '000000', '654321', '121212', '112233', '123123', '666666',
  'password', 'password1', 'passw0rd', 'pass', 'passwort', 'contrasena',
  'qwerty', 'qwertyuiop', 'qwerty123', 'asdfgh', 'asdfghjkl', 'zxcvbn',
  'zxcvbnm', '1q2w3e4r', '1qaz2wsx', 'qazwsx', 'qwe123',
  'iloveyou', 'princess', 'sunshine', 'shadow', 'monkey', 'dragon', 'football',
  'baseball', 'basketball', 'soccer', 'hockey', 'superman', 'batman',
  'pokemon', 'starwars', 'letmein', 'welcome', 'login', 'admin', 'administrator',
  'root', 'toor', 'guest', 'user', 'test', 'demo', 'default',
  'abc123', 'a1b2c3', 'trustno1', 'whatever', 'freedom', 'ninja', 'jordan',
  'harley', 'ranger', 'hunter', 'buster', 'thomas', 'tigger', 'robert',
  'michael', 'jennifer', 'jessica', 'daniel', 'ashley', 'charlie', 'andrew',
  'matthew', 'joshua', 'george', 'summer', 'winter', 'autumn', 'spring',
  'flower', 'cookie', 'chocolate', 'computer', 'internet', 'samsung', 'google',
  'apple', 'amazon', 'microsoft', 'linkedin', 'facebook', 'twitter',
  'money', 'secret', 'silver', 'orange', 'purple', 'yellow', 'banana',
  'cheese', 'coffee', 'guitar', 'hello', 'love', 'lovely', 'forever',
  'nothing', 'access', 'master', 'mustang', 'corvette', 'ferrari', 'diamond',
  'phoenix', 'killer', 'chelsea', 'liverpool', 'arsenal', 'barcelona',
  // App-specific guesses.
  'xquakshell', 'quakshell', 'masterpassword', 'vault', 'vaultpassword',
  'changeme', 'ssh', 'sshkey', 'terminal', 'console',
]);

// Physical rows used to spot keyboard walks such as "asdfgh". Shift/AltGr
// layouts are ignored on purpose: the point is to catch the obvious walk, not
// to model every keyboard in existence.
const KEYBOARD_ROWS = ['1234567890', 'qwertyuiop', 'asdfghjkl', 'zxcvbnm'];

interface KeyPosition {
  row: number;
  col: number;
}

const KEY_POSITIONS: ReadonlyMap<string, KeyPosition> = (() => {
  const map = new Map<string, KeyPosition>();
  KEYBOARD_ROWS.forEach((row, rowIndex) => {
    for (let col = 0; col < row.length; col++) {
      map.set(row[col], { row: rowIndex, col });
    }
  });
  return map;
})();

/**
 * Evaluates the requirement checklist shown under the password field.
 * Independent of the scoring below: only `minLength` blocks submission.
 */
export function checkPasswordRequirements(password: string): PasswordChecklist {
  const chars = [...password];
  return {
    minLength: chars.length >= MIN_MASTER_PASSWORD_LENGTH,
    recommendedLength: chars.length >= RECOMMENDED_MASTER_PASSWORD_LENGTH,
    hasLower: /[a-z]/.test(password),
    hasUpper: /[A-Z]/.test(password),
    hasDigit: /[0-9]/.test(password),
    hasSymbol: /[^a-zA-Z0-9]/.test(password),
  };
}

/** Estimates how much guessing work a password represents. */
export function evaluatePasswordStrength(password: string): StrengthResult {
  const chars = [...password];
  const warnings: string[] = [];

  if (chars.length === 0) {
    return { entropyBits: 0, score: 0, label: 'weak', warnings };
  }

  const bitsPerChar = Math.log2(characterPoolSize(password));
  let bits = chars.length * bitsPerChar;

  const lower = password.toLowerCase();

  if (isCommonPassword(lower)) {
    // A listed password is guessed in seconds regardless of how long it is.
    bits = Math.min(bits, 12);
    warnings.push('This is a commonly used password.');
  } else {
    const match = longestCommonSubstring(lower);
    if (match && match.length * 2 >= chars.length) {
      bits -= 0.6 * bitsPerChar * match.length;
      warnings.push(`Built around the common word "${match}".`);
    }
  }

  const repeat = repeatedCharPenalty(chars, bitsPerChar);
  if (repeat > 0) {
    bits -= repeat;
    warnings.push('Contains repeated characters.');
  }

  const block = repeatedBlock(password);
  if (block) {
    // A repeated block is only as hard to guess as the block itself, so this
    // replaces the length-based estimate rather than shaving it.
    bits = bitsPerChar * [...block.unit].length + Math.log2(block.times);
    warnings.push('Repeats the same fragment over and over.');
  }

  // A run counted as a character sequence must not be charged again as a
  // keyboard walk; "12345" is both, and one penalty is the honest one.
  const covered = new Array<boolean>(chars.length).fill(false);

  const sequence = sequencePenalty(chars, bitsPerChar, covered);
  if (sequence > 0) {
    bits -= sequence;
    warnings.push('Contains a character sequence such as "abcd" or "4321".');
  }

  const keyboard = keyboardPenalty(chars, bitsPerChar, covered);
  if (keyboard > 0) {
    bits -= keyboard;
    warnings.push('Contains a keyboard pattern such as "qwerty".');
  }

  if (/(?:19|20)\d{2}$/.test(password)) {
    bits -= 6;
    warnings.push('Ends with a year, which attackers try first.');
  }

  const classes = countClasses(password);
  bits *= CLASS_FACTOR[classes];
  // Anything short of a single alphabet is left to the checklist below the
  // meter, which already spells out which kinds of character are missing.
  if (classes === 1) {
    warnings.push('Uses only one kind of character.');
  }

  bits = Math.max(0, bits);

  return {
    entropyBits: bits,
    score: clamp(Math.round((bits / FULL_BAR_BITS) * 100), 0, 100),
    label: weakest(bandFor(bits, chars.length), ceilingFor(classes, chars.length)),
    warnings,
  };
}

function bandFor(bits: number, length: number): StrengthLabel {
  // Below the hard floor nothing else matters; the submit button is disabled
  // anyway, and a green meter there would be actively misleading.
  if (length < MIN_MASTER_PASSWORD_LENGTH) return 'weak';
  if (bits < MEDIUM_BITS) return 'weak';
  if (bits < STRONG_BITS) return 'medium';
  return 'strong';
}

/**
 * The best verdict a password may reach given how many character classes it
 * uses.
 *
 * Raw entropy rewards length alone, so a long enough string of digits would
 * otherwise read as strong — which is exactly the advice people should not be
 * given. Narrow alphabets are also what wordlist and mask attacks target first.
 * Length still buys its way up, it just has to buy more.
 */
function ceilingFor(classes: number, length: number): StrengthLabel {
  if (classes >= 3) return 'strong';
  if (classes === 2) return length >= 16 ? 'strong' : 'medium';
  if (length >= 24) return 'strong';
  return length >= 14 ? 'medium' : 'weak';
}

const ORDER: StrengthLabel[] = ['weak', 'medium', 'strong'];

function weakest(a: StrengthLabel, b: StrengthLabel): StrengthLabel {
  return ORDER.indexOf(a) <= ORDER.indexOf(b) ? a : b;
}

function characterPoolSize(password: string): number {
  let pool = 0;
  if (/[a-z]/.test(password)) pool += 26;
  if (/[A-Z]/.test(password)) pool += 26;
  if (/[0-9]/.test(password)) pool += 10;
  if (/[!-\/:-@[-`{-~]/.test(password)) pool += 32; // printable ASCII punctuation
  if (/[^\x00-\x7F]/.test(password)) pool += 64;
  return Math.max(pool, 2);
}

function countClasses(password: string): number {
  let classes = 0;
  if (/[a-z]/.test(password)) classes++;
  if (/[A-Z]/.test(password)) classes++;
  if (/[0-9]/.test(password)) classes++;
  if (/[^a-zA-Z0-9]/.test(password)) classes++;
  return classes;
}

function isCommonPassword(lower: string): boolean {
  if (COMMON_PASSWORDS.has(lower)) return true;
  // "password123" and "password!" are the same guess as "password".
  const stripped = lower.replace(/[!.]+$/, '').replace(/\d{1,4}$/, '');
  return stripped !== lower && COMMON_PASSWORDS.has(stripped);
}

function longestCommonSubstring(lower: string): string | null {
  let best: string | null = null;
  for (const entry of COMMON_PASSWORDS) {
    if (entry.length < 4) continue;
    if (!lower.includes(entry)) continue;
    if (!best || entry.length > best.length) best = entry;
  }
  return best;
}

function repeatedCharPenalty(chars: string[], bitsPerChar: number): number {
  let penalty = 0;
  let run = 1;
  for (let i = 1; i <= chars.length; i++) {
    if (i < chars.length && chars[i] === chars[i - 1]) {
      run++;
      continue;
    }
    if (run >= 3) penalty += 0.75 * bitsPerChar * (run - 1);
    run = 1;
  }
  return penalty;
}

function repeatedBlock(password: string): { unit: string; times: number } | null {
  const length = password.length;
  for (let unitLength = 1; unitLength <= length / 2; unitLength++) {
    if (length % unitLength !== 0) continue;
    const unit = password.slice(0, unitLength);
    const times = length / unitLength;
    if (unit.repeat(times) === password) return { unit, times };
  }
  return null;
}

function sequencePenalty(chars: string[], bitsPerChar: number, covered: boolean[]): number {
  return runPenalty(chars, bitsPerChar, covered, 3, (a, b) => {
    const delta = b.codePointAt(0)! - a.codePointAt(0)!;
    return delta === 1 || delta === -1 ? delta : null;
  });
}

// Four keys, not three: three-key walks such as "tre" or "asd" turn up inside
// ordinary words often enough that charging them would flag good passwords.
function keyboardPenalty(chars: string[], bitsPerChar: number, covered: boolean[]): number {
  return runPenalty(chars, bitsPerChar, covered, 4, (a, b) => {
    const from = KEY_POSITIONS.get(a.toLowerCase());
    const to = KEY_POSITIONS.get(b.toLowerCase());
    if (!from || !to || from.row !== to.row) return null;
    const delta = to.col - from.col;
    return delta === 1 || delta === -1 ? delta : null;
  });
}

/**
 * Charges runs of `minRun` or more characters that step by a constant ±1 under
 * `step`. Indices already charged by an earlier pass are skipped so a run is
 * never penalised twice.
 */
function runPenalty(
  chars: string[],
  bitsPerChar: number,
  covered: boolean[],
  minRun: number,
  step: (a: string, b: string) => number | null,
): number {
  let penalty = 0;
  let start = 0;
  let direction: number | null = null;

  const flush = (end: number) => {
    const length = end - start;
    if (direction !== null && length >= minRun && !covered.slice(start, end).some(Boolean)) {
      penalty += 0.75 * bitsPerChar * (length - 1);
      for (let i = start; i < end; i++) covered[i] = true;
    }
  };

  for (let i = 1; i <= chars.length; i++) {
    const delta = i < chars.length ? step(chars[i - 1], chars[i]) : null;
    if (delta !== null && (direction === null || delta === direction)) {
      direction = delta;
      continue;
    }
    flush(i);
    start = delta === null ? i : i - 1;
    direction = delta;
  }

  return penalty;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
