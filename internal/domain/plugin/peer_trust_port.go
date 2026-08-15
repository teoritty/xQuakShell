package plugin

import "context"

// PeerTrustInboundPort is what a plugin may ask the core about trusting a remote identity.
//
// The port is deliberately narrow: one question, a yes/no answer. A plugin has no need to tell
// "not known" from "the material changed" - the human makes the decision and the core chooses the
// wording. The less a plugin is told, the fewer reasons it has to build its own logic on it.
//
// The scope (which plugin the trust belongs to) is absent from the parameters: the caller supplies
// it from the session binding, so naming someone else's is impossible.
type PeerTrustInboundPort interface {
	VerifyPeer(ctx context.Context, pluginID, sessionID, subject string, material []byte) (bool, error)
}
