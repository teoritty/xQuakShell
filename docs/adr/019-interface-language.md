# ADR-019: Interface Language

## Status

Accepted

## Context

The interface was written in English throughout: roughly seven hundred strings across a hundred
Svelte components, plus the string tables that feed the settings search, the file panel toolbar and
the transfer conflict vocabulary. Nothing in the application knew what a language was.

Three things about this codebase decide the shape of any answer:

- **Settings live inside the encrypted vault** (`domain.AppSettings` → `vault.age`). The screen that
  asks for the master password is drawn before any of it is readable, and it is the one screen a
  user who cannot read English most needs translated.
- **The application is portable** (ADR-006). There is no `%APPDATA%`; everything the user can add
  lives under `<exe>/data`.
- **Plugins produce words the user reads.** Discovery node labels, action captions, the confirmation
  text on a destructive action — none of it is ours, and none of it is in any catalogue we hold.

## Decision

### 1. The catalogue is owned by Go, layered, and merged before it reaches the interface

English and Russian are compiled into the binary (`//go:embed`), and any `<code>.json` dropped into
`<exe>/data/locales` is laid on top. `Pack(code)` returns English, then the built-in pack for that
language, then the on-disk one, already merged — so a key a translation omits arrives as its English
text and no caller implements a fallback of its own.

Rejected: bundling the catalogues in the frontend and reading only user packs through Go. It puts
the merge rule in two places, and the one in TypeScript cannot be covered by the Go tests that
already gate this repository.

### 2. Keys under `security.` come from the built-in packs only

A pack in `<exe>/data/locales` is written by anyone who can reach that folder, and its strings go
straight into the interface. Host key verification, remote identity verification, plugin install
consent, plugin trust policy, secret logging and every irreversible delete are decided by a user
reading a warning. A pack able to rewrite *the host key changed — this may be a machine-in-the-middle
attack* into a reassurance would turn a translation file into a way to talk someone through accepting
a forged server.

So `overlay` refuses an untrusted pack the right to **replace** a key under `domain.SecurityMessagePrefix`.
It may still **add** one English does not claim, which is what lets a language with no built-in pack
be translated in full; and a Go test fails if any security key exists in a translation but not in
English, because a warning that lives only in a translation is one a disk pack could have supplied.

Rejected: forbidding translation of those strings outright. It would leave a Russian interface with
English warnings — exactly the strings a user most needs to understand — to prevent an attack the
override ban already prevents.

### 3. Translated text is never rendered as markup

`interpolate` produces a string and nothing else; no branch of the i18n layer builds HTML, and no
`{@html}` receives a translated value. Sentences that used to wrap a hostname in `<code>` or a verb
in `<strong>` now interpolate it instead — which a translation needs anyway, since a language is free
to put the variable somewhere else in the sentence.

### 4. The language code is mirrored outside the vault

The vault stays the source of truth. The code alone — not a secret, and a UI preference of the kind
§5.4 T5 already permits there — is mirrored into `localStorage`, read before the app mounts, and
reconciled with the vault the moment it unlocks. Without it the master password prompt is always
English.

### 5. Plugins are told the language; they translate themselves

A new capability, `i18n` 1.0.0, with one feature flag, `localeChanged`. A plugin that declares it
receives `locale` in its `initialize` handshake and an `i18n.localeChanged` notification on every
change — and again whenever it starts, because a plugin that came back after a change would
otherwise keep writing in the language it first launched under. Level-triggered and repeated on
start, the same contract `discovery.observe` uses (ADR-014) and for the same reason.

`capabilities.i18n.locales` is informational. The host does not validate it, does not withhold the
notification for a language absent from it, and does not read an empty list as "supports nothing" —
a plugin translating itself from a source the host cannot see has nothing honest to put there.

It is its own capability rather than a `ui` feature because a plugin with no surface still hands the
host words, and should not have to claim the right to draw in order to learn what language to write
them in. It grants no privilege — a language tag and nothing else — so it raises no install-time
consent.

Rejected: publishing the language on the existing event bus as `core.ui.locale`. It needs no API
change at all, but it makes the language one anonymous item in a stream a plugin must opt into by
pattern, with nothing in the manifest saying which plugins claim to be translated.

Rejected: the host reading a plugin's own locale files and handing it a finished dictionary. It
forces one file format on every plugin, in every language, and buys nothing the plugin could not do
itself.

### 6. Errors, logs and audit stay English

They are not translated and are not routed through the catalogue. A message the backend produced is
shown as the backend wrote it — including a plugin compatibility issue, which names capability
versions the frontend cannot parse. Inventing a translation for a string this layer cannot read
would be guessing at what the backend meant, and an audit record whose wording depends on a file in
`data/locales` is not a record.

## Consequences

- A user adds a language by dropping one JSON file next to the executable; no rebuild, no plugin.
- A translator sees every string in one place per language, including the settings search terms.
- Security warnings cannot be reworded by anything on disk, in any language.
- Plugin authors get the language for free at startup and on every change, with one code path.
- The English pack is load-bearing: a key missing from it renders as its own key. The pack parity
  test is what keeps that honest.

## References

- ADR-006 — portable data layout, written up in [security-model.md](../security-model.md#portable-data-layout-adr-006)
- [ADR-007](007-host-filesystem-trust.md) — filesystem zones
- [ADR-012](012-plugin-api-versioning.md) — capability versioning, additive-only rule
- [ADR-014](014-discovery-subtrees.md) — the level-triggered notification this one copies
- [ADR-017](017-release-and-compatibility-policy.md) — what makes a contract change additive
