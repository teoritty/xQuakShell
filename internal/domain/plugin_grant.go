package domain

import (
	"slices"
	"time"
)

// PluginGrant records the permissions a user consented to for one plugin (ADR-022).
//
// It replaces asking "was this plugin id granted secret access, yes or no". A grant keyed by a name
// survives the bundle behind that name being replaced, and it says nothing about how much was asked
// for, so a plugin could widen what it requests without anyone being asked again.
//
// Granted holds opaque permission tokens. This package stores them and does not interpret them: the
// vocabulary belongs to internal/domain/plugin, which derives a set from a manifest and decides
// whether a later manifest asks for more. Keeping the meaning out of here is what lets the two
// packages stay unaware of each other.
type PluginGrant struct {
	PluginID string `json:"pluginId"`
	// Granted is the permission set the user agreed to, as stored tokens.
	Granted []string `json:"granted"`
	// GrantedAt is when consent was given, so an audit can answer when rather than only whether.
	GrantedAt time.Time `json:"grantedAt"`
}

// cloneGrant copies a grant and the slice inside it. Both directions need this: a grant handed out
// must not be a window into stored consent, and a grant taken in must not share an array the caller
// goes on using.
func cloneGrant(in PluginGrant) PluginGrant {
	out := in
	out.Granted = cloneStrings(in.Granted)
	return out
}

// GrantFor returns the consent recorded for a plugin, or false when there is none.
//
// Absence is not an error: it is a plugin nobody has consented to yet, and turning that into one
// would force every caller to tell "never asked" apart from "storage broke". It is also not the same
// as an empty grant - "never consented" and "consented to nothing" lead to different dialogs.
func (p *PluginSettings) GrantFor(pluginID string) (PluginGrant, bool) {
	if p == nil || pluginID == "" {
		return PluginGrant{}, false
	}
	for _, grant := range p.PluginGrants {
		if grant.PluginID == pluginID {
			return cloneGrant(grant), true
		}
	}
	return PluginGrant{}, false
}

// RecordGrant creates or REPLACES the consent recorded for a plugin.
//
// Replaces, precisely: two grants for one plugin would mean either one counts as consent, and
// re-consenting to a narrower set would leave the wider one standing and doing the deciding.
func (p *PluginSettings) RecordGrant(grant PluginGrant) {
	if p == nil || grant.PluginID == "" {
		return
	}
	stored := cloneGrant(grant)
	for i := range p.PluginGrants {
		if p.PluginGrants[i].PluginID == grant.PluginID {
			p.PluginGrants[i] = stored
			return
		}
	}
	p.PluginGrants = append(p.PluginGrants, stored)
}

// revokeGrant drops the recorded consent and reports whether there was any to drop, so the caller
// can log what the user is losing rather than announcing a revocation that removed nothing.
func (p *PluginSettings) revokeGrant(pluginID string) bool {
	before := len(p.PluginGrants)
	p.PluginGrants = slices.DeleteFunc(p.PluginGrants, func(grant PluginGrant) bool {
		return grant.PluginID == pluginID
	})
	return len(p.PluginGrants) != before
}
