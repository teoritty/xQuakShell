package discovery

import "time"

// Limits are pinned by ADR-014 and are not overridable by a plugin manifest or by
// configuration: they exist to bound a hostile or buggy plugin's ability to grow unbounded
// state or UI in the host process, not to be tuned per deployment.
const (
	// MaxDepth bounds how many group levels a plugin may nest. 8 comfortably covers real
	// hierarchies (e.g. docker > containers > <container>) while capping recursive rendering
	// and recursive icon-inheritance walks (ResolveIcon) to a fixed cost.
	MaxDepth = 8

	// MaxChildrenPerPublish bounds one discovery.publish snapshot. Exceeding it is truncation
	// (see TruncateChildren), not a rejected publish — the user should see a large-but-partial
	// list and know it's partial, rather than see nothing because a plugin enumerated too much.
	MaxChildrenPerPublish = 500

	// MaxNodesPerPlugin bounds total nodes held in host memory per (plugin, CONNECTION), across
	// every branch that plugin has published into. ADR-014 writes the limit as per (plugin,
	// session) because a session is what a plugin can see; the host stores one tree per connection
	// and enforces it there, which is the same budget from the only side that owns the memory.
	// It is the backstop against a plugin that stays under the per-publish cap but keeps many
	// branches "loading" forever.
	MaxNodesPerPlugin = 2000

	// MaxIDLen bounds Node.ID and Action.ID. IDs are plugin-chosen opaque strings (often a
	// remote resource ID), not host-generated, so they need a hard ceiling independent of any
	// particular backend's own ID format.
	MaxIDLen = 256

	// MaxLabelLen bounds Node.Label and Action.Label. 128 runes is generous for a resource name
	// or a menu entry while keeping a single row from dominating the tree's fixed-width layout.
	MaxLabelLen = 128

	// MaxTooltipLen bounds Status.Tooltip. Tooltips are free text a plugin author writes for a
	// human, not an identifier, so the ceiling is looser than MaxLabelLen but still bounded —
	// an unbounded tooltip is an unbounded DOM string rendered on hover.
	MaxTooltipLen = 256

	// MaxConfirmLen bounds Action.Confirm, on the same reasoning as MaxTooltipLen: a sentence
	// written for a human, and the one piece of plugin text shown in a MODAL the user must read
	// before a destructive action. An unbounded one is a dialog with no dismiss button on screen.
	MaxConfirmLen = 256

	// MaxActionsPerNode bounds Node.Actions. A context menu with more than 16 entries stops
	// being a menu; this is a UI sanity limit, not a security one.
	MaxActionsPerNode = 16

	// MaxNodesPerInvoke bounds nodeIds in one discovery.invokeAction, matching the mass-action
	// model (ADR-014 "Actions"): a large but finite batch, not an unbounded fan-out that could
	// turn one click into thousands of remote operations.
	MaxNodesPerInvoke = 200

	// MaxPublishPerSecond bounds inbound discovery.publish requests per (plugin, CONNECTION) — not
	// per session. A connection shows one subtree however many tabs are open, and the session
	// carrying the traffic can change under it during a leader handover, so a per-session budget
	// would silently reset every time the transport moved. It caps the CPU/allocation cost of
	// processing snapshots from a plugin that polls too aggressively, independent of
	// CoalesceInterval, which caps frontend *render* churn.
	MaxPublishPerSecond = 20

	// CoalesceInterval is how long the frontend batches per-node publish events before
	// re-rendering. It exists because a plugin may legitimately publish the same branch
	// several times in quick succession (e.g. streaming enumeration progress); without
	// coalescing, each one would force a separate tree re-render.
	CoalesceInterval = 100 * time.Millisecond

	// InvokeAckTimeout bounds how long the host waits for a plugin to acknowledge
	// discovery.invokeAction. The ack only confirms receipt, not completion — actual results
	// arrive later via an ordinary publish (ADR-014 "Actions") — so 5s is a receipt timeout,
	// not a work timeout, and can stay short.
	InvokeAckTimeout = 5 * time.Second
)
