# xQuakShell — portable Windows archive

Everything in this folder belongs to one installation. Unpack it wherever you like — a user folder,
a USB stick, a network share — and run `xQuakShell.exe` from there. There is no installer, nothing
is written to the registry, and nothing is left behind anywhere else on the machine.

## Which archive is this

| Archive | Contains | Use it when |
|---------|----------|-------------|
| `…-windows-amd64-portable.zip` | The application only | The machine has the Microsoft WebView2 runtime — Windows 11 and most up-to-date Windows 10 installations do |
| `…-windows-amd64-portable-webview2.zip` | The application and the WebView2 runtime (`WebView2\`) | Clean, offline or locked-down machines, or when the smaller archive does not start |

xQuakShell draws its interface with the Microsoft WebView2 runtime. The larger archive carries a
fixed version of that runtime in the `WebView2` folder next to the executable and uses it directly:
nothing is installed and no download is needed. Keep the folder next to `xQuakShell.exe` — moving
or renaming it makes the application fall back to a runtime installed on the system, and fail if
there is none.

Both archives are 64-bit (amd64).

## "Windows protected your PC"

Expect this on first run, and expect Microsoft Defender to call the app an unrecognised program.

xQuakShell is not signed with a code-signing certificate, so SmartScreen has no publisher to check
and no download history to weigh. That is a statement about the certificate, not about the file:
every unsigned application from a small publisher gets the same screen.

To run it anyway: **More info** → **Run anyway**.

Verify the download first if you would rather not take that on trust. Each release publishes
`SHA256SUMS`; in PowerShell, from the folder holding the archive:

```powershell
Get-FileHash .\xQuakShell-<version>-windows-amd64-portable.zip -Algorithm SHA256
```

and compare the result with the matching line in `SHA256SUMS`.

If the file was already unpacked, Windows may keep the download mark on it. `Unblock-File` clears
that:

```powershell
Get-ChildItem -Recurse | Unblock-File
```

## Where your data lives

In `data\`, next to the executable:

- `data\vault.age` — the encrypted vault: connections, credentials, keys
- `data\audit\` — the audit log
- `data\plugins\` — installed plugins and their own data

Back up `data\`, or copy the whole folder, and your setup moves with it. Deleting this folder
removes the application and its data together.

## If nothing happens when you start it

The application needs a working WebView2 runtime, and until it has one it cannot show a window.
Download the `-portable-webview2` archive: it carries its own and needs no installation. If that
one also does nothing, the folder was probably unpacked without the `WebView2` folder beside the
executable — unpack the archive again, keeping its structure.

---

Documentation and source: https://github.com/teoritty/xQuakShell
