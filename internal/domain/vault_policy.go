package domain

// MinMasterPasswordLength is the hard floor for a *new* master password, counted
// in runes rather than bytes so a multi-byte character counts once.
//
// It is enforced in VaultRepository.Create only. Unlock deliberately skips the
// check: an existing vault must stay openable with whatever password created
// it, including one created before this policy existed.
//
// The frontend duplicates this number in
// frontend/src/lib/vault/passwordStrength.ts (MIN_MASTER_PASSWORD_LENGTH) to
// disable the submit button early. That duplication is deliberate — there is no
// Go-to-TypeScript constant generation in this repository — so the two must be
// changed together.
const MinMasterPasswordLength = 8
