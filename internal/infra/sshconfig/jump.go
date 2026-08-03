package sshconfig

import (
	"strings"

	"xquakshell/internal/domain"
)

// maxJumpHops bounds a ProxyJump chain. OpenSSH allows arbitrary nesting via
// hops that themselves declare ProxyJump; the limit keeps a mutually
// referencing pair of hosts from producing an unbounded chain.
const maxJumpHops = 8

// resolveJumpChain expands a host's ProxyJump into an ordered hop list.
//
// The chain is built breadth-first through the config: each hop named in a
// ProxyJump is itself resolved against the configuration, so `ProxyJump
// bastion` inherits the `Host bastion` block's HostName, User, Port and
// IdentityFile, and a bastion that itself declares ProxyJump contributes its
// own hop ahead of itself — matching the order ssh(1) traverses.
func (r *resolver) resolveJumpChain(alias string, s hostSettings) []domain.SSHConfigHop {
	if s.proxyJump == "" || strings.EqualFold(s.proxyJump, "none") {
		return nil
	}
	visited := map[string]bool{strings.ToLower(alias): true}
	return r.expandJump(s.proxyJump, alias, visited, 0)
}

// expandJump parses one ProxyJump value (which may list several comma-
// separated hops) into hops, recursing into each hop's own ProxyJump.
func (r *resolver) expandJump(value, subject string, visited map[string]bool, depth int) []domain.SSHConfigHop {
	if depth >= maxJumpHops {
		r.notices.add(domain.SSHConfigNoticeLimitReached, subject)
		return nil
	}
	var hops []domain.SSHConfigHop
	for _, spec := range strings.Split(value, ",") {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		hop, ok := r.buildHop(spec, subject, visited, depth)
		if !ok {
			continue
		}
		hops = append(hops, hop...)
		if len(hops) >= maxJumpHops {
			r.notices.add(domain.SSHConfigNoticeLimitReached, subject)
			return hops[:maxJumpHops]
		}
	}
	return hops
}

// buildHop resolves a single `[user@]host[:port]` specification, prefixed by
// any hops that specification itself depends on.
func (r *resolver) buildHop(spec, subject string, visited map[string]bool, depth int) ([]domain.SSHConfigHop, bool) {
	parsed, ok := parseJumpSpec(spec)
	if !ok {
		r.notices.add(domain.SSHConfigNoticeJumpHostUnresolved, subject)
		return nil, false
	}
	key := strings.ToLower(parsed.alias)
	if visited[key] {
		// A cycle: the chain would revisit a host already on the path.
		r.notices.add(domain.SSHConfigNoticeJumpHostUnresolved, subject)
		return nil, false
	}
	visited[key] = true

	settings := r.resolveSettings(parsed.alias)
	var prefix []domain.SSHConfigHop
	if settings.proxyJump != "" && !strings.EqualFold(settings.proxyJump, "none") {
		prefix = r.expandJump(settings.proxyJump, parsed.alias, visited, depth+1)
	}
	return append(prefix, r.hopFrom(parsed, settings)), true
}

// hopFrom merges an inline jump specification with the configuration block of
// the same name. Values written inline win: `ProxyJump admin@bastion:2222` is
// a deliberate override of whatever `Host bastion` says.
func (r *resolver) hopFrom(parsed jumpSpec, settings hostSettings) domain.SSHConfigHop {
	hostName := effectiveHostName(settings.hostName, parsed.alias)
	hop := domain.SSHConfigHop{
		Alias:    parsed.alias,
		HostName: hostName,
		Port:     firstNonZero(parsed.port, settings.port),
		User:     firstNonEmpty(parsed.user, settings.user),
	}
	hop.IdentityFiles = r.resolveIdentityFiles(settings.identityFiles, identityTokens{
		hostName: hop.HostName,
		alias:    parsed.alias,
		user:     hop.User,
	}, parsed.alias)
	return hop
}

// jumpSpec is a parsed `[user@]host[:port]` token.
type jumpSpec struct {
	user  string
	alias string
	port  int
}

// parseJumpSpec splits a jump specification, tolerating IPv6 literals in
// brackets (`[2001:db8::1]:2222`) whose colons must not be read as a port
// separator.
func parseJumpSpec(spec string) (jumpSpec, bool) {
	var out jumpSpec
	if at := strings.LastIndex(spec, "@"); at >= 0 {
		out.user = spec[:at]
		spec = spec[at+1:]
	}
	hostPart, portPart := splitHostPort(spec)
	out.alias = strings.TrimSpace(hostPart)
	if out.alias == "" {
		return jumpSpec{}, false
	}
	if portPart != "" {
		if out.port = parsePort(portPart); out.port == 0 {
			return jumpSpec{}, false
		}
	}
	return out, true
}

// splitHostPort separates an optional trailing :port from a host token.
func splitHostPort(spec string) (string, string) {
	if strings.HasPrefix(spec, "[") {
		if end := strings.Index(spec, "]"); end >= 0 {
			host := spec[1:end]
			rest := spec[end+1:]
			return host, strings.TrimPrefix(rest, ":")
		}
		return spec, ""
	}
	// A bare IPv6 literal has several colons and no port; only a single colon
	// can be a port separator.
	if strings.Count(spec, ":") != 1 {
		return spec, ""
	}
	host, port, _ := strings.Cut(spec, ":")
	return host, port
}

func firstNonZero(values ...int) int {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
