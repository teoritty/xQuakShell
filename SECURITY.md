# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | ✅ |
| Anything older | ❌ |

Only the most recent release receives fixes, security fixes included. There are no long-lived
release branches and no backports: a single maintainer promising a support window they cannot meet
would leave people on an old build believing they were covered, which for a tool holding SSH keys is
worse than promising nothing. Fixes ship as a new release.

**There is no fixed response deadline.** Timing follows severity. A critical fix can be cut from the
last release tag without waiting for unrelated work in `main` to be finished.

The application checks GitHub Releases once the vault is unlocked and tells you when a newer version
exists, because "upgrade to the latest release" is only a usable answer if you can find out one came
out. The check waits for the unlock deliberately: the setting that permits it lives inside the
encrypted vault, so before then the application cannot know whether you allowed the request. It is
one anonymous GET of a public endpoint, it downloads and installs nothing, it is recorded in the
audit log, and it can be turned off under Settings -> About.

The full policy, including what counts as a breaking change to each published contract, is
[ADR-017](docs/adr/017-release-and-compatibility-policy.md).

## Reporting a Vulnerability

If you discover a security vulnerability in xQuakShell, please report it responsibly.

**Do not** open a public issue for security vulnerabilities.

Instead:

1. **Email** the maintainer at **fedorlockytov2014@gmail.com** with details of the vulnerability.
2. Include steps to reproduce, affected versions, and potential impact.
3. Allow reasonable time for a fix before public disclosure.

We will acknowledge receipt and work on a fix. We appreciate your effort to help keep xQuakShell and its users safe.

## Security Considerations

- **Vault:** All sensitive data is encrypted with age (scrypt). The master password is never stored; it is used only to derive the decryption key.
- **Keys and passwords:** Stored encrypted in the vault. Passphrases are cached in memory and cleared on lock/exit.
- **Host keys:** Strict verification; no auto-accept of unknown or mismatched keys.
- **Logs:** No secrets (passwords, keys, vault contents) are logged.

If you have questions about security design rather than a vulnerability to report, open a normal
issue.
