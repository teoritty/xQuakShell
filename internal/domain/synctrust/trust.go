package synctrust

import (
	"bytes"
	"encoding/json"
	"slices"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
)

// Action is what the engine does with an incoming version (spec §6.3).
type Action int

const (
	// ActionApply applies the version on arrival.
	ActionApply Action = iota + 1
	// ActionPending keeps the local value in force until the user confirms with the master
	// password, which never crosses to another device or into the WebView.
	ActionPending
	// ActionRefuse never applies the version; the record is quarantined and reported.
	ActionRefuse
)

// Verdict is an Action with the reason shown to the user when the change is held or refused.
type Verdict struct {
	Action Action
	Reason string
}

var applyVerdict = Verdict{Action: ActionApply}

func pending(reason string) Verdict { return Verdict{Action: ActionPending, Reason: reason} }

// Classify decides whether an incoming version may weaken this device. local is the version this
// device holds (nil when it has none), incoming is the remote version (nil for a deletion). Both are
// record data as Snapshot produces it. An error means the data does not decode and the record goes to
// quarantine.
//
// A change is held when it widens what something else may do with this device's keys or plugins,
// because another device - or whoever holds the sync password - must not be able to do from afar
// what the master password guards here.
func Classify(t RecordType, local, incoming json.RawMessage) (Verdict, error) {
	if section, ok := IsSettingsType(t); ok && incoming == nil {
		return Verdict{Action: ActionRefuse, Reason: "a settings section cannot be deleted: " + section}, nil
	}
	if incoming == nil {
		return classifyDeletion(t, local), nil
	}
	switch t {
	case SettingsType("plugins"):
		return classifyPlugins(local, incoming)
	case SettingsType("lockout"):
		return classifyLockout(local, incoming)
	case TypeIdentity:
		return classifyIdentity(local, incoming)
	case TypeConnection:
		return classifyConnection(local, incoming)
	case TypeKnownHost:
		return classifyReplacement(local, incoming, hostAnchor, "replaces the stored key of a known host")
	case TypePeerTrust:
		return classifyReplacement(local, incoming, peerAnchor, "replaces the peer identity a plugin trusts")
	}
	return applyVerdict, nil
}

// anchorDeletions names the records whose deletion waits for the master password, with the reason
// shown. Deleting a host key, a peer identity or a key narrows nothing by itself, but it is the
// first half of replacing one: once the stored version is gone, the next version of the same item
// arrives as a new one, and a new item applies on arrival. Holding the replacement and applying the
// deletion let any device of the account - or a team editor - swap a host key, swap the identity a
// plugin trusts, or clear a key's non-exportable latch in two commits instead of one.
var anchorDeletions = map[RecordType]string{
	TypeKnownHost: "removes the stored key of a known host",
	TypePeerTrust: "removes the peer identity a plugin trusts",
	TypeIdentity:  "removes a key together with its restrictions",
}

// classifyDeletion applies every other deletion: a deleted grant never widens anything. A deletion
// of something this device does not hold has nothing to keep in force.
func classifyDeletion(t RecordType, local json.RawMessage) Verdict {
	if reason, ok := anchorDeletions[t]; ok && local != nil {
		return pending(reason)
	}
	return applyVerdict
}

func decodeOrZero[T any](raw json.RawMessage, zero T) (T, error) {
	if raw == nil {
		return zero, nil
	}
	return decodeData[T](raw)
}

func classifyPlugins(local, incoming json.RawMessage) (Verdict, error) {
	def := pluginsOf(domain.AppSettings{Plugins: domain.DefaultPluginSettings()})
	l, err := decodeOrZero(local, def)
	if err != nil {
		return Verdict{}, err
	}
	in, err := decodeData[pluginsSection](incoming)
	if err != nil {
		return Verdict{}, err
	}
	var lp, ip domain.AppSettings
	applyPlugins(&lp, l)
	applyPlugins(&ip, in)
	if domain.PluginTrustWeakened(lp.Plugins, ip.Plugins) {
		return pending("weakens plugin trust"), nil
	}
	return applyVerdict, nil
}

// idleLimit treats a non-positive timeout as no timeout at all, which is the weakest setting.
func idleLimit(d time.Duration) time.Duration {
	if d <= 0 {
		return time.Duration(1<<63 - 1)
	}
	return d
}

func classifyLockout(local, incoming json.RawMessage) (Verdict, error) {
	l, err := decodeOrZero(local, domain.LockoutSettings{})
	if err != nil {
		return Verdict{}, err
	}
	in, err := decodeData[domain.LockoutSettings](incoming)
	if err != nil {
		return Verdict{}, err
	}
	if !l.Enabled {
		return applyVerdict, nil
	}
	switch {
	case !in.Enabled:
		return pending("disables the automatic lock"), nil
	case idleLimit(in.IdleTimeout) > idleLimit(l.IdleTimeout):
		return pending("lengthens the automatic lock timeout"), nil
	case l.LockOnMinimize && !in.LockOnMinimize:
		return pending("stops locking on minimise"), nil
	}
	return applyVerdict, nil
}

func classifyIdentity(local, incoming json.RawMessage) (Verdict, error) {
	in, err := decodeData[identityData](incoming)
	if err != nil {
		return Verdict{}, err
	}
	if local == nil {
		return applyVerdict, nil
	}
	l, err := decodeData[identityData](local)
	if err != nil {
		return Verdict{}, err
	}
	li, ii := l.Identity, in.Identity
	switch {
	case li.NonExportable && !ii.NonExportable:
		return Verdict{Action: ActionRefuse, Reason: "makes a non-exportable key exportable"}, nil
	case !li.AllowPlugins && ii.AllowPlugins:
		return pending("lets plugins read a private key"), nil
	case !li.AllowForwarding && ii.AllowForwarding:
		return pending("allows a key to be forwarded"), nil
	}
	return applyVerdict, nil
}

func classifyConnection(local, incoming json.RawMessage) (Verdict, error) {
	in, err := decodeData[domain.Connection](incoming)
	if err != nil {
		return Verdict{}, err
	}
	l, err := decodeOrZero(local, domain.Connection{})
	if err != nil {
		return Verdict{}, err
	}
	if in.AgentForwarding == nil || !in.AgentForwarding.Enabled {
		return applyVerdict, nil
	}
	if l.AgentForwarding == nil || !l.AgentForwarding.Enabled {
		return pending("enables agent forwarding"), nil
	}
	for _, id := range in.AgentForwarding.IdentityIDs {
		if !slices.Contains(l.AgentForwarding.IdentityIDs, id) {
			return pending("forwards an additional key"), nil
		}
	}
	return applyVerdict, nil
}

// classifyReplacement holds a version that changes an existing trust anchor. A new anchor applies on
// arrival with its provenance shown; replacing one is what an attacker who wants to sit in the middle
// would need, so it waits for the person at the machine. anchor extracts what is trusted - the line,
// the material - so a change to anything else, such as when it was added, is not a replacement.
func classifyReplacement[T any](local, incoming json.RawMessage, anchor func(T) []byte, reason string) (Verdict, error) {
	in, err := decodeData[T](incoming)
	if err != nil {
		return Verdict{}, err
	}
	if local == nil {
		return applyVerdict, nil
	}
	l, err := decodeData[T](local)
	if err != nil {
		return Verdict{}, err
	}
	if bytes.Equal(anchor(l), anchor(in)) {
		return applyVerdict, nil
	}
	return pending(reason), nil
}

func hostAnchor(k knownHostData) []byte { return []byte(k.Line) }

func peerAnchor(e domain.PeerTrustEntry) []byte { return e.Material }

var credentialFields = map[RecordType][]string{
	TypePassword:     {"value"},
	TypePluginSecret: {"value"},
	TypeKnownHost:    {"line"},
	TypePeerTrust:    {"material"},
}

var connectionCredentials = []string{"host", "port", "jumpChain", "agentForwarding"}
var userCredentials = []string{"authMethod", "keyAuth", "passAuth", "pluginAuth"}
var identityCredentials = []string{"policy", "allowPlugins", "allowForwarding", "nonExportable"}

// FieldPolicyFor classifies merge conflicts for one record type (plan table "credential-class
// fields"): two devices that set a password or a host to different values get a conflict record for
// a person to resolve, never a silent winner.
func FieldPolicyFor(t RecordType) vaultsync.FieldPolicy {
	return func(path []string) vaultsync.FieldClass {
		if isCredential(t, path) {
			return vaultsync.FieldCredential
		}
		return vaultsync.FieldOrdinary
	}
}

func isCredential(t RecordType, path []string) bool {
	if len(path) == 0 {
		return false
	}
	switch t {
	case TypeConnection:
		if path[0] == "users" {
			return len(path) > 2 && slices.Contains(userCredentials, path[2])
		}
		return slices.Contains(connectionCredentials, path[0])
	case TypeIdentity:
		return path[0] == "blob" || (path[0] == "identity" && len(path) > 1 && slices.Contains(identityCredentials, path[1]))
	}
	return slices.Contains(credentialFields[t], path[0])
}
