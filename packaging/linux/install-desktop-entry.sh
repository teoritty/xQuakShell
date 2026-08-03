#!/bin/sh
# Registers the unpacked archive with the desktop environment, and undoes it with --remove.
#
# Nothing here installs the application: the archive stays where it was unpacked and keeps all of
# its state beside the binary. This only writes a .desktop file that points at that location, which
# is what puts xQuakShell in the application menu with its icon and associates the running window
# with it. Deleting the archive directory and running this script with --remove leaves no trace.
#
# The entry is written per-user rather than into /usr/share/applications: a portable archive that
# needed root to be useful would not be portable, and a system-wide entry would point at one user's
# home directory anyway.
set -eu

here=$(dirname "$(readlink -f "$0")")
template="$here/xquakshell.desktop"
apps_dir="${XDG_DATA_HOME:-$HOME/.local/share}/applications"
target="$apps_dir/xquakshell.desktop"

# Best-effort refresh: menus of some desktop environments only notice a new entry after the cache
# is rebuilt, but the entry is valid either way, so a missing or failing tool is not an error.
refresh_menu() {
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$apps_dir" >/dev/null 2>&1 || true
	fi
}

if [ "${1:-}" = "--remove" ]; then
	rm -f "$target"
	refresh_menu
	echo "Removed $target"
	exit 0
fi

if [ "${1:-}" != "" ]; then
	echo "usage: $(basename "$0") [--remove]" >&2
	exit 2
fi

if [ ! -f "$template" ]; then
	echo "$(basename "$0"): $template is missing; run this from the unpacked archive." >&2
	exit 1
fi

mkdir -p "$apps_dir"

# awk rather than sed: the unpack directory is arbitrary user input and would otherwise have to be
# escaped against whatever sed delimiter was chosen. gsub still treats & in the replacement as "the
# matched text", so that one character is escaped explicitly.
awk -v dir="$here" '
	BEGIN { gsub(/&/, "\\\\&", dir) }
	{ gsub(/%%INSTALL_DIR%%/, dir); print }
' "$template" >"$target"

chmod 644 "$target"
refresh_menu

echo "Installed $target"
echo "xQuakShell should now appear in your application menu."
echo "To undo: $(basename "$0") --remove"
