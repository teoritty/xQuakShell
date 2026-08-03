#!/bin/sh
# Launcher for the portable Linux archive.
#
# Two things make this wrapper worth shipping instead of pointing the desktop entry straight at
# the executable:
#
#   - The app resolves its data directory from os.Executable() (ADR-006): the vault, the audit log
#     and every installed plugin live next to the binary. readlink -f keeps that true when the
#     launcher is reached through a symlink in ~/.local/bin or through a desktop entry, and exec
#     hands the process over so the binary's own path is what the app sees.
#   - Wails links the system WebKitGTK, which cannot be bundled. When it is missing, the dynamic
#     loader prints "cannot open shared object file" and exits — started from an application menu,
#     with no terminal attached, that is a window that never appears and no visible reason why.
#     The check below turns it into an instruction the user can act on.
set -eu

here=$(dirname "$(readlink -f "$0")")
bin="$here/xQuakShell"

if [ ! -x "$bin" ]; then
	echo "xQuakShell: $bin is missing or not executable." >&2
	echo "If the archive was unpacked by a tool that drops permissions, run: chmod +x \"$bin\"" >&2
	exit 1
fi

# ldd is absent on some minimal systems, and a check that cannot run must never stop the app from
# starting: only a definite "not found" is treated as a failure.
if command -v ldd >/dev/null 2>&1; then
	missing=$(ldd "$bin" 2>/dev/null | awk '/not found/ { print $1 }')
	if [ -n "$missing" ]; then
		# Which WebKitGTK this archive was built against is visible in the soname it asks for, so
		# the advice can name the right package instead of listing every possibility.
		case "$missing" in
		*webkit2gtk-4.1*)
			apt_pkg="libwebkit2gtk-4.1-0"
			dnf_pkg="webkit2gtk4.1"
			pacman_pkg="webkit2gtk-4.1"
			other_archive="webkit4.0"
			;;
		*webkit2gtk-4.0*)
			apt_pkg="libwebkit2gtk-4.0-37"
			dnf_pkg="webkit2gtk3"
			pacman_pkg="webkit2gtk"
			other_archive="webkit4.1"
			;;
		*)
			apt_pkg=""
			;;
		esac

		echo "xQuakShell cannot start: required system libraries are missing." >&2
		echo "$missing" | sed 's/^/  /' >&2
		if [ -n "$apt_pkg" ]; then
			echo >&2
			echo "Install the WebKitGTK runtime this archive was built against:" >&2
			echo "  Debian/Ubuntu:  sudo apt install $apt_pkg" >&2
			echo "  Fedora:         sudo dnf install $dnf_pkg" >&2
			echo "  Arch:           sudo pacman -S $pacman_pkg" >&2
			echo "  other:          your distribution's package for the same library" >&2
			echo >&2
			echo "If it is not available on your system, download the -$other_archive archive of" >&2
			echo "the same release instead — it is the same application built against the other" >&2
			echo "WebKitGTK version." >&2
		fi
		exit 1
	fi
fi

exec "$bin" "$@"
