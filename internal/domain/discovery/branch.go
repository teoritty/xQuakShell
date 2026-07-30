package discovery

// BranchState is the host's own view of one node's children, as distinct from anything a
// plugin says about the node itself (Status). "loading"/"ready"/"error" describe the last
// discovery.publish the host processed; "stale" describes a host-local condition — the
// leading session handed over — that no plugin message caused and no plugin should be able to
// claim.
type BranchState string

const (
	BranchLoading BranchState = "loading"
	BranchReady   BranchState = "ready"
	BranchError   BranchState = "error"
	// BranchStale marks a branch during leading-session handover (ADR-014 "Leading session").
	// It exists only in host memory; a plugin publishing it would be lying about a host-only
	// fact, which is why PluginMayPublishState excludes it.
	BranchStale BranchState = "stale"
)

// Truncated records that a publish exceeded MaxChildrenPerPublish and was cut down by the
// host. It is host-computed (see TruncateChildren) and never something a plugin sends —
// a plugin claiming truncation it didn't actually trigger would misrepresent how complete
// the list the user is looking at really is.
type Truncated struct {
	Shown int `json:"shown"`
	Total int `json:"total"`
}

// Branch is the host-owned state of one expanded node's children: is the last publish
// still loading, did it succeed, did it error, and (if truncated) how much was cut.
//
// The split between Branch and Node/Status is normative (ADR-014): the host never writes
// into Status — that would silently overwrite the plugin's own reported health with the
// host's transport-level view. The plugin never controls Branch.State or Truncated — that
// would let a plugin claim staleness or truncation the host never observed. Each side owns
// exactly the fields it can honestly speak to.
type Branch struct {
	State     BranchState `json:"state"`
	Error     string      `json:"error,omitempty"`
	Truncated *Truncated  `json:"truncated,omitempty"`
}

// PluginMayPublishState reports whether s is a state a plugin's discovery.publish snapshot may
// legitimately carry. Only loading/ready/error describe something the plugin actually
// observed about its own work; stale describes a host-side handover the plugin has no way to
// know about, so it is excluded here rather than merely undocumented.
func PluginMayPublishState(s BranchState) bool {
	switch s {
	case BranchLoading, BranchReady, BranchError:
		return true
	default:
		return false
	}
}
