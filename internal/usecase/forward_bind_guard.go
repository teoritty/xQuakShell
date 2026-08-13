package usecase

import (
	"fmt"
	"net"

	"xquakshell/internal/domain"
)

// verifyLoopbackListener closes ln and fails when it did not actually land on a loopback address.
//
// domain.IsLoopbackBind validates the string the rule carries, and a string is not a fact. It
// accepts the literal "localhost", which the resolver - not this process - decides the meaning of:
// a line in the hosts file, or a resolver that answers for it, points "localhost" at an external
// interface, and a forward the validator called loopback-only listens to the local network. The
// same is true of any name that happens to resolve to a routable address.
//
// So the promise is checked against what the kernel bound rather than what the user typed. This is
// the second half of a defence in depth: the validator keeps a bad rule from being saved, and this
// keeps a rule that passed validation from listening anywhere it said it would not.
func verifyLoopbackListener(ln net.Listener, ruleID string) error {
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		// Not a TCP listener, so this rule is not one of the kinds the loopback promise covers.
		return nil
	}
	if addr.IP.IsLoopback() {
		return nil
	}
	if err := ln.Close(); err != nil {
		return fmt.Errorf("%w: bound to %s; closing the listener also failed: %v",
			domain.ErrForwardBindNotLoopback, addr.IP, err)
	}
	return fmt.Errorf("%w: rule %s bound to %s", domain.ErrForwardBindNotLoopback, ruleID, addr.IP)
}
