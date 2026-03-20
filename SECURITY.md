# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in xQuakShell, please report it responsibly.

**Do not** open a public issue for security vulnerabilities.

Instead:

1. **Email** the maintainer(s) (see `wails.json` or project metadata for contact) with details of the vulnerability.
2. Include steps to reproduce, affected versions, and potential impact.
3. Allow reasonable time for a fix before public disclosure.

We will acknowledge receipt and work on a fix. We appreciate your effort to help keep xQuakShell and its users safe.

## Security Considerations

- **Vault:** All sensitive data is encrypted with age (scrypt). The master password is never stored; it is used only to derive the decryption key.
- **Keys and passwords:** Stored encrypted in the vault. Passphrases are cached in memory and cleared on lock/exit.
- **Host keys:** Strict verification; no auto-accept of unknown or mismatched keys.
- **Logs:** No secrets (passwords, keys, vault contents) are logged.

If you have questions about security design, open a general discussion (not a vulnerability report).
