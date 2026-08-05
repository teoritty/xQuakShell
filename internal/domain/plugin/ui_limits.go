package plugin

import "time"

// UI limits (ADR-015 §Limits). Every number a plugin can push against lives here, in one file,
// because the value and the reason for it are the same fact: a limit whose rationale is three
// packages away from its constant is a limit the next reader will "tidy up".
const (
	// MaxSurfacesPerPluginDefault applies when a manifest declares no maxSurfaces. Eight is what a
	// resource browser needs in practice — a handful of log tabs and a shell — while staying small
	// enough that a misbehaving plugin cannot bury the tab bar before anyone notices.
	MaxSurfacesPerPluginDefault = 8

	// MaxSurfacesPerPluginCeiling bounds what a manifest may ask for. The declaration is the
	// plugin's own estimate and is trusted only up to here: past this point the cost is paid by a
	// user who never sees the manifest.
	MaxSurfacesPerPluginCeiling = 16

	// MaxSurfaceTitleLen matches discovery's Label budget (ADR-014). A tab title and a tree row
	// are the same kind of string in the same window, and two different ceilings for them would be
	// a difference with no meaning behind it.
	MaxSurfaceTitleLen = 128

	// MaxLogSurfaceBytes and MaxLogSurfaceLines bound one log surface's ring buffer. The buffer is
	// the reason a log surface can offer search and export at all, and a bound is the reason it
	// cannot become an unattended memory leak on a chatty container. Whichever is hit first wins;
	// past either, the oldest content is dropped and the viewer says so rather than quietly
	// pretending the log started later than it did.
	MaxLogSurfaceBytes = 8 << 20
	MaxLogSurfaceLines = 200000

	// MaxOpenDialogsPerPlugin is one. A second modal from the same plugin would stack over the
	// first, and the user would answer a question without seeing which one they were answering.
	MaxOpenDialogsPerPlugin = 1

	// MaxDialogFields bounds one dialog or details panel. A form past this size is a report, and a
	// report belongs in a detail dialog's code block, not in a hundred input elements.
	MaxDialogFields = 100

	// MaxKeyValuePairs bounds one keyValue field. Labels and driver options are the motivating
	// cases and neither has a legitimate hundred-entry form.
	MaxKeyValuePairs = 64

	// MaxCodeFieldBytes matches the NDJSON frame cap: a code block larger than the frame carrying
	// it could never arrive whole, so a larger limit here would only move the failure.
	MaxCodeFieldBytes = 256 << 10
)

// NodeDetailsAckTimeout bounds discovery.describeNode and discovery.applyDetails, matching the
// invokeAction budget (ADR-014): both are host->plugin requests that must acknowledge receipt
// promptly and report the real outcome by publishing afterwards.
const NodeDetailsAckTimeout = 5 * time.Second
