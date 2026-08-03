import {
  MIN_MASTER_PASSWORD_LENGTH,
  checkPasswordRequirements,
  evaluatePasswordStrength,
} from './passwordStrength';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

function run() {
  // --- bands ---------------------------------------------------------------

  {
    const r = evaluatePasswordStrength('');
    assert(r.score === 0 && r.entropyBits === 0, 'empty password scores zero');
    assert(r.label === 'weak', 'empty password is weak');
    assert(r.warnings.length === 0, 'empty password produces no warnings to shout at the user');
  }

  {
    // Below the hard floor the band is weak no matter what the estimate says.
    const r = evaluatePasswordStrength('Xk7#pQ');
    assert(r.label === 'weak', 'a password below the minimum length is always weak');
  }

  {
    const r = evaluatePasswordStrength('kestrelzu9');
    assert(r.label === 'medium', 'a mid-entropy two-class password lands in the medium band');
  }

  // --- character classes cap the verdict -----------------------------------

  {
    // Length alone must not buy a verdict: a digit-only password is what mask
    // attacks target first, so it is held down however long it gets.
    const digits = [
      { pw: '9382716450', expected: 'weak' },
      { pw: '93827164503948572610', expected: 'medium' },
      { pw: '9382716450394857261049382716', expected: 'medium' },
    ];
    for (const { pw, expected } of digits) {
      const r = evaluatePasswordStrength(pw);
      assert(r.label === expected, `${pw.length} digits is ${expected}, got ${r.label}`);
    }
    assert(
      evaluatePasswordStrength('9382716450394857261049382716').warnings.some((w) => w.includes('one kind')),
      'a digit-only password says why it is held back',
    );
  }

  {
    // Two classes need real length before they reach strong.
    assert(evaluatePasswordStrength('kestrelzulu9382').label === 'medium', 'fifteen characters of two classes stays medium');
    assert(evaluatePasswordStrength('kestrelzulu93827').label === 'strong', 'sixteen characters of two classes may be strong');
  }

  {
    // A same-length password using more classes always rates at least as well.
    const one = evaluatePasswordStrength('kestrelzuluxray');
    const three = evaluatePasswordStrength('Kestrelzulu9382');
    assert(three.entropyBits > one.entropyBits, 'more character classes means more estimated entropy');
  }

  {
    // The meter is an entropy estimate, not a guessability oracle: a long,
    // four-class password with no detected pattern reads as strong.
    const r = evaluatePasswordStrength('correct-horse-battery-staple-9!');
    assert(r.label === 'strong', 'a long four-class passphrase is strong');
    assert(r.score === 100, 'a very high estimate fills the meter');
    assert(r.warnings.length === 0, 'a clean passphrase raises no warnings');
  }

  // --- penalties -----------------------------------------------------------

  {
    const r = evaluatePasswordStrength('1234567');
    assert(r.label === 'weak', 'a seven character password is weak');
    assert([...'1234567'].length === MIN_MASTER_PASSWORD_LENGTH - 1, 'the fixture is one short of the floor');
  }

  {
    const r = evaluatePasswordStrength('password');
    assert(r.label === 'weak', 'a listed password is weak');
    assert(r.warnings.some((w) => w.includes('commonly used')), 'a listed password says so');
  }

  {
    // Trailing digits do not rescue a listed password.
    const r = evaluatePasswordStrength('sunshine12');
    assert(r.label === 'weak', 'a listed password with a digit suffix is still weak');
    assert(r.warnings.some((w) => w.includes('commonly used')), 'the digit suffix is stripped before the lookup');
  }

  {
    const r = evaluatePasswordStrength('aaaaaaaaaaaa');
    assert(r.label === 'weak', 'a single repeated character is weak however long it is');
    assert(r.warnings.some((w) => w.includes('fragment')), 'a repeated unit is reported');
  }

  {
    const r = evaluatePasswordStrength('abcabcabcabc');
    assert(r.label === 'weak', 'a repeated block is weak however long it is');
    assert(r.warnings.some((w) => w.includes('fragment')), 'the repeated fragment is reported');
  }

  {
    const r = evaluatePasswordStrength('abcdefghij');
    assert(r.label === 'weak', 'an alphabet run is weak');
    assert(r.warnings.some((w) => w.includes('sequence')), 'the sequence is reported');
  }

  {
    const r = evaluatePasswordStrength('qwertyuiop');
    assert(r.label === 'weak', 'a keyboard row is weak');
    assert(r.warnings.some((w) => w.includes('keyboard')), 'the keyboard walk is reported');
  }

  {
    // "12345678" is both a digit run and a keyboard walk; it must be charged once.
    const r = evaluatePasswordStrength('12345678');
    const patternWarnings = r.warnings.filter((w) => w.includes('sequence') || w.includes('keyboard'));
    assert(patternWarnings.length === 1, 'a run is charged as either a sequence or a keyboard walk, never both');
  }

  {
    const r = evaluatePasswordStrength('Kestrel2024');
    assert(r.warnings.some((w) => w.includes('year')), 'a trailing year is reported');
    assert(
      r.entropyBits < evaluatePasswordStrength('Kestrel2j4b').entropyBits,
      'a trailing year costs entropy relative to the same-length alternative',
    );
  }

  {
    const single = evaluatePasswordStrength('bqxmvzkw');
    assert(single.warnings.some((w) => w.includes('one kind')), 'a single-class short password is reported');
    assert(
      single.entropyBits < evaluatePasswordStrength('bqxmvzkW').entropyBits,
      'adding a second character class raises the estimate',
    );
  }

  {
    // Three-key walks such as "tre" turn up inside ordinary words; charging
    // them would flag perfectly good passwords.
    assert(
      !evaluatePasswordStrength('kestrelzu9').warnings.some((w) => w.includes('keyboard')),
      'a three-key walk inside a word is not called a keyboard pattern',
    );
    assert(
      evaluatePasswordStrength('poiuytrewq').warnings.some((w) => w.includes('keyboard')),
      'a long keyboard walk still is',
    );
  }

  // --- monotonicity --------------------------------------------------------

  {
    const pairs: Array<[string, string]> = [
      ['bqxmvzkw', 'bqxmvzkwj'],
      ['summer-day', 'summer-day-fog'],
      ['Xk7#pQ', 'Xk7#pQv'],
    ];
    for (const [shorter, longer] of pairs) {
      assert(
        evaluatePasswordStrength(longer).score >= evaluatePasswordStrength(shorter).score,
        `extending ${shorter} never lowers the score`,
      );
    }
  }

  // --- checklist -----------------------------------------------------------

  {
    const empty = checkPasswordRequirements('');
    assert(!empty.minLength && !empty.recommendedLength, 'empty meets no length requirement');
    assert(!empty.hasLower && !empty.hasUpper && !empty.hasDigit && !empty.hasSymbol, 'empty meets no class requirement');

    const floor = checkPasswordRequirements('abcdefgh');
    assert(floor.minLength && !floor.recommendedLength, 'eight characters meets the floor but not the recommendation');
    assert(floor.hasLower && !floor.hasUpper && !floor.hasDigit && !floor.hasSymbol, 'lowercase only is detected');

    const all = checkPasswordRequirements('Abcdefgh1234!');
    assert(all.minLength && all.recommendedLength, 'thirteen characters meets both length requirements');
    assert(all.hasLower && all.hasUpper && all.hasDigit && all.hasSymbol, 'all four classes are detected');

    // Code points, not UTF-16 units: an emoji must not count as two characters.
    const emoji = checkPasswordRequirements('🔐🔐🔐🔐🔐🔐🔐');
    assert(!emoji.minLength, 'seven astral code points do not reach the eight character floor');
  }

  console.log('passwordStrength.test.ts passed');
}

try {
  run();
} catch (e) {
  console.error(e);
  process.exit(1);
}
