# Environment Workarounds

Collected workarounds for host environments where xQuakShell fails through no fault of its own:
a graphics driver, a system library, a compositor, a WebView runtime. Each entry names the exact
output you see, what is actually broken, the workaround, and why it is not applied to everyone by
default.

This document exists because such failures are indistinguishable from application bugs at the point
where a user meets them — the window never appears, or the process dies with a message about a
protocol or a buffer — and the searchable text of that message is the only thing the user has to go
on. So every entry quotes it verbatim.

Two things this document is not:

- **Not a list of application bugs.** A bug in xQuakShell is fixed, not documented here. Everything
  below is a defect or a limitation in something the application links against and cannot bundle.
- **Not a place for "try these flags and see".** An entry is added once the cause is understood and
  the workaround confirmed on a real machine. An unverified guess belongs in an issue.

---

## Linux: blank window, or `Error 71` on Wayland, on the proprietary NVIDIA driver

### What you see

Under a Wayland session the process dies immediately:

```
Overriding existing handler for signal 10. Set JSC_SIGNAL_FOR_GC if you want WebKit to use a different signal
Gdk-Message: Error 71 (Protocol error) dispatching to Wayland display.
```

Under X11 or XWayland it survives but no window ever appears, and this repeats:

```
Failed to create GBM buffer of size 1280x800: Invalid argument
```

The first line about signal 10 is WebKit claiming a signal for its garbage collector. It is normal
and appears on every healthy Linux start too — it is not part of the failure.

### What is actually broken

WebKitGTK 2.46 and newer render through a DMA-BUF path by default, allocating its buffers through
GBM. The proprietary NVIDIA driver does not provide the buffer formats that path asks for, so every
allocation fails with `EINVAL`.

The two messages are the same failure seen from two distances. Under X11 the failed allocation stays
inside WebKit, which retries forever and composites nothing. Under Wayland the invalid buffer is
committed to the compositor, which rejects it and tears down the connection — and a Wayland protocol
error is fatal by design, so the process exits.

The reason this matters for diagnosis: the error text points at Wayland and GDK, so the obvious first
move is to change the display backend — and it does not help, because GTK and GDK are working
correctly in both cases. The breakage is a layer below them, inside WebKit's renderer, and it is
reached identically whichever backend is in use. `GDK_BACKEND=x11` converts a crash into a blank
window and nothing more.

This is not specific to xQuakShell. Every WebKitGTK application on such a machine is affected, and
nothing about it is reachable from application code: Wails links the system WebKitGTK, which cannot
be bundled and whose renderer is chosen before any of our code runs.

### Workaround

```sh
WEBKIT_DISABLE_DMABUF_RENDERER=1 ./xquakshell.sh
```

To keep it, put it in the `Exec=` line of the desktop entry that
[`install-desktop-entry.sh`](../packaging/linux/install-desktop-entry.sh) wrote to
`~/.local/share/applications/xquakshell.desktop`:

```
Exec=env WEBKIT_DISABLE_DMABUF_RENDERER=1 /path/to/xquakshell.sh
```

To fix every WebKitGTK application on the machine at once, export it from a shell file that a
graphical session reads — `~/.zshenv` or `~/.profile`. `~/.zshrc` and `~/.bashrc` are the wrong
place: an application started from a menu has no interactive shell, so neither file is read and the
variable that works in a terminal is absent exactly where the launcher needs it.

**Do not also set `WEBKIT_DISABLE_COMPOSITING_MODE=1`.** It is a separate, heavier switch that turns
off accelerated compositing entirely, and it makes terminal scrolling visibly worse. It is worth
trying only if the DMA-BUF variable alone leaves the window blank, which means a different cause
than this entry.

### Why the launcher does not set it automatically

It could: the proprietary driver is detectable from `/sys/module/nvidia_drm`, and the variable could
be set when the user has not set it. That was considered and declined. The variable is a WebKitGTK
setting, not an application setting, and a launcher that quietly rewrites the graphics environment
of every NVIDIA user would also be papering over the day WebKit and the driver start working
together — leaving everyone on the slow path with nothing in the output to suggest why. The
detection heuristic would then be load-bearing, permanent, and wrong.

### Confirmed on

| | |
|---|---|
| Distribution | Arch Linux, kernel 7.2.3 |
| Session | Wayland |
| GPU | GeForce GTX 1060 3GB, `nvidia` kernel driver (proprietary) |
| Packages | `webkit2gtk-4.1` 2.52.6-1, `mesa` 1:26.2.2-1 |
| Build | `xQuakShell-v1.4.0-linux-amd64-webkit4.1` |
| Date | 2026-09-20 |

`mesa` is listed for completeness and is not involved: with the proprietary driver loaded, GL and GBM
go through NVIDIA's own implementation.

---

## Adding an entry

Keep the five headings above — *What you see*, *What is actually broken*, *Workaround*, *Why … not
… automatically*, *Confirmed on* — and drop the fourth when nothing was ever proposed to be
automatic.

Quote the output exactly as the user saw it, including the lines that turn out to be irrelevant, and
say which ones those are. A user matching their terminal against this document does not yet know
which line is the signal.

*Confirmed on* is the part that ages. An entry whose workaround is no longer needed on current
versions is worth a note saying so, with the version that fixed it, rather than deletion: someone
running the older combination still needs it.
