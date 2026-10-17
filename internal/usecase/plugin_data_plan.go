package usecase

// pluginFileAction is what reconciling one synced plugin file does.
type pluginFileAction int

const (
	actionNone pluginFileAction = iota
	// actionPush copies the disk's version into the vault mirror, from where sync carries it.
	actionPush
	// actionWrite copies the mirror's version onto the disk; only while the plugin is stopped.
	actionWrite
	// actionWait is an incoming version held back because the plugin is running.
	actionWait
)

// planPluginFile decides one file from three digests, "" meaning absent: the file on disk now, the
// vault mirror (what sync last agreed on), and what this device last wrote or read on disk.
//
// A disk that moved since this device last touched it is a local edit, and it goes out even when the
// mirror moved too: two edits of one file resolve last-writer-wins, and the version this push
// replaces stays in the vault's history for the trash window. Only an untouched disk takes the
// mirror's version, and only while the plugin cannot be reading or writing it.
func planPluginFile(disk, mirror, last string, mayWrite bool) (pluginFileAction, bool) {
	if disk != last {
		conflict := mirror != last && mirror != disk
		return actionPush, conflict
	}
	if mirror == disk {
		return actionNone, false
	}
	if mayWrite {
		return actionWrite, false
	}
	return actionWait, false
}
