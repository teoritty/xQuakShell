package recovery

import (
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// A key that is not full length, or that leans on characters outside the alphabet, is a key with
// less entropy than advertised. Every one of these has to be refused outright rather than padded,
// truncated, or accepted with a shrug.
func TestOnlyAFullLengthKeyInTheAlphabetIsAccepted(t *testing.T) {
	valid := strings.Repeat("A", domain.RecoveryKeyLength)

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"canonical", valid, true},
		{"one short", valid[:len(valid)-1], false},
		{"one long", valid + "A", false},
		{"empty", "", false},
		{"only dashes", strings.Repeat("-", 39), false},
		{"U is excluded from the alphabet", strings.Repeat("A", 31) + "U", false},
		{"a character outside base32", strings.Repeat("A", 31) + "!", false},
		{"whitespace padded to the right length", strings.Repeat(" ", 32), false},
	}

	for _, tc := range cases {
		if _, ok := domain.NormalizeRecoveryKey(tc.input); ok != tc.want {
			t.Errorf("%s: accepted = %v, want %v; anything but a full 160-bit key is a weaker credential than the one promised", tc.name, ok, tc.want)
		}
	}
}

// Everything a person adds while copying a key off paper has to fold away, or the feature fails the
// one moment it exists for. The confusable letters fold to digits because Crockford excludes them
// from the alphabet precisely so that a misread is recoverable.
func TestTranscriptionMistakesFoldToTheSameKey(t *testing.T) {
	canonical := "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	variants := map[string]string{
		"grouped with dashes": domain.FormatRecoveryKey(canonical),
		"lowercase":           strings.ToLower(canonical),
		"spaces":              "0123 4567 89AB CDEF GHJK MNPQ RSTV WXYZ",
		"pasted with newline": canonical + "\n",
		"tab separated":       "0123\t456789ABCDEFGHJKMNPQRSTVWXYZ",
		"O read as zero":      "O123456789ABCDEFGHJKMNPQRSTVWXYZ",
		"I read as one":       "0I23456789ABCDEFGHJKMNPQRSTVWXYZ",
		"l read as one":       "0l23456789ABCDEFGHJKMNPQRSTVWXYZ",
	}

	for name, variant := range variants {
		got, ok := domain.NormalizeRecoveryKey(variant)
		if !ok {
			t.Errorf("%s: rejected outright", name)
			continue
		}
		if got != canonical {
			t.Errorf("%s: normalized to %q, want %q; a user who reads their own handwriting slightly wrong must still get in", name, got, canonical)
		}
	}
}

// The fold is only safe because the characters it folds away never appear in a generated key. If
// EncodeRecoveryKey ever emitted an O, an I or an L, two distinct keys would normalize to one and
// the search space would quietly shrink.
func TestNoGeneratedKeyContainsAFoldedCharacter(t *testing.T) {
	for i := range 500 {
		key, err := vault.NewRecoveryKey()
		if err != nil {
			t.Fatalf("generate %d: %v", i, err)
		}
		if strings.ContainsAny(key, "OILU") {
			t.Fatalf("generated key %q contains a folded or excluded character; two keys would now normalize to one", key)
		}
		if len(key) != domain.RecoveryKeyLength {
			t.Fatalf("generated key %q is %d characters, want %d", key, len(key), domain.RecoveryKeyLength)
		}
		if strings.Trim(key, crockford) != "" {
			t.Fatalf("generated key %q leaves the alphabet", key)
		}
	}
}

// A generator that repeats is a generator that is not reading the entropy source. Five thousand
// draws from 2^160 collide with probability far below any flake threshold, so a duplicate here
// means the randomness is broken, not that the test was unlucky.
func TestGeneratedKeysDoNotRepeat(t *testing.T) {
	const draws = 5000
	seen := make(map[string]struct{}, draws)

	for i := range draws {
		key, err := vault.NewRecoveryKey()
		if err != nil {
			t.Fatalf("generate %d: %v", i, err)
		}
		if _, dup := seen[key]; dup {
			t.Fatalf("draw %d repeated %q; the entropy source is not being read", i, key)
		}
		seen[key] = struct{}{}
	}
}

// Every character position has to vary. A generator with a stuck five-bit group would still pass the
// uniqueness test above while quietly losing entropy at that position.
func TestEveryPositionOfAGeneratedKeyVaries(t *testing.T) {
	const draws = 400
	seenAt := make([]map[rune]struct{}, domain.RecoveryKeyLength)
	for i := range seenAt {
		seenAt[i] = map[rune]struct{}{}
	}

	for range draws {
		key, err := vault.NewRecoveryKey()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		for i, r := range key {
			seenAt[i][r] = struct{}{}
		}
	}

	// With 400 draws over a 32-character alphabet, a healthy position shows well over half of it.
	// Twenty is far below that and far above anything a stuck position could reach.
	for i, seen := range seenAt {
		if len(seen) < 20 {
			t.Errorf("position %d took only %d distinct values in %d draws; that position is carrying less than five bits", i, len(seen), draws)
		}
	}
}

// EncodeRecoveryKey must refuse the wrong amount of entropy rather than encode it. Silently
// encoding ten bytes would produce a key that looks right and is half the strength.
func TestEncodeRefusesTheWrongAmountOfEntropy(t *testing.T) {
	for _, size := range []int{0, 1, domain.RecoveryKeyBytes - 1, domain.RecoveryKeyBytes + 1, 64} {
		if got := domain.EncodeRecoveryKey(make([]byte, size)); got != "" {
			t.Errorf("%d bytes encoded to %q; only %d bytes may produce a key", size, got, domain.RecoveryKeyBytes)
		}
	}
}

// The encoding has to be injective, or two different vault keys could be wrapped under the same
// printed credential.
func TestEncodingIsInjectiveOverSingleBitChanges(t *testing.T) {
	base := make([]byte, domain.RecoveryKeyBytes)
	baseKey := domain.EncodeRecoveryKey(base)

	for byteIndex := range base {
		for bit := range 8 {
			flipped := make([]byte, domain.RecoveryKeyBytes)
			copy(flipped, base)
			flipped[byteIndex] ^= 1 << uint(bit)

			if got := domain.EncodeRecoveryKey(flipped); got == baseKey {
				t.Fatalf("byte %d bit %d encodes to the same key as the zero value; a bit of entropy is being dropped", byteIndex, bit)
			}
		}
	}
}
