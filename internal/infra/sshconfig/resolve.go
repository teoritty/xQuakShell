package sshconfig

import (
	"strconv"
	"strings"

	"xquakshell/internal/domain"
)

// maxHosts bounds how many connections one config can offer to import. A
// generated config with thousands of entries is legitimate; rendering and
// saving all of them is not something the user meant to ask for in one click.
const maxHosts = 5000

// hostSettings is the resolved value of every directive the importer honours,
// for one host alias.
type hostSettings struct {
	hostName        string
	user            string
	port            int
	identityFiles   []string
	proxyJump       string
	hasProxyCommand bool
}

// resolver answers questions about a parsed configuration.
type resolver struct {
	blocks  []block
	homeDir string
	notices *noticeSet
}

// resolveSettings folds every block that applies to alias into one settings
// value, following OpenSSH precedence: for scalar keywords the first value
// obtained wins, and IdentityFile accumulates across all matching blocks in
// the order they appear.
func (r *resolver) resolveSettings(alias string) hostSettings {
	var s hostSettings
	for _, b := range r.blocks {
		if !b.matches(alias) {
			continue
		}
		for _, d := range b.settings {
			r.applyDirective(&s, d)
		}
	}
	return s
}

// applyDirective applies one directive to the settings being accumulated.
func (r *resolver) applyDirective(s *hostSettings, d directive) {
	switch d.keyword {
	case "hostname":
		setFirst(&s.hostName, firstArg(d))
	case "user":
		setFirst(&s.user, firstArg(d))
	case "port":
		if s.port == 0 {
			s.port = parsePort(firstArg(d))
		}
	case "identityfile":
		s.identityFiles = append(s.identityFiles, d.args...)
	case "proxyjump":
		setFirst(&s.proxyJump, firstArg(d))
	case "proxycommand":
		s.hasProxyCommand = true
	}
}

// aliases lists the concrete host names declared anywhere in the config, in
// file order and without duplicates.
func (r *resolver) aliases() []string {
	var (
		out  []string
		seen = map[string]bool{}
	)
	for _, b := range r.blocks {
		if b.isMatch {
			r.notices.add(domain.SSHConfigNoticeMatchBlockSkipped, "")
			continue
		}
		if b.isDefaultsOnly() {
			continue
		}
		for _, alias := range b.concreteAliases() {
			key := strings.ToLower(alias)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, alias)
			if len(out) >= maxHosts {
				r.notices.add(domain.SSHConfigNoticeLimitReached, "")
				return out
			}
		}
	}
	return out
}

// buildHost turns one alias into a fully resolved importable host.
func (r *resolver) buildHost(alias string) domain.SSHConfigHost {
	s := r.resolveSettings(alias)
	hostName := effectiveHostName(s.hostName, alias)
	host := domain.SSHConfigHost{
		Alias:    alias,
		HostName: hostName,
		Port:     s.port,
		User:     s.user,
		IdentityFiles: r.resolveIdentityFiles(s.identityFiles, identityTokens{
			hostName: hostName,
			alias:    alias,
			user:     s.user,
		}, alias),
	}
	host.JumpHops = r.resolveJumpChain(alias, s)
	if s.proxyJump == "" && s.hasProxyCommand {
		r.notices.add(domain.SSHConfigNoticeProxyCommandUnsupported, alias)
	}
	return host
}

// effectiveHostName applies the %h token and the alias fallback.
//
// %h inside HostName expands to the alias, which is how configs like
// `Host *.internal` / `HostName %h.example.com` are written.
func effectiveHostName(declared, alias string) string {
	if declared == "" {
		return alias
	}
	return strings.ReplaceAll(declared, "%h", alias)
}

// parsePort accepts only a valid TCP port; anything else leaves the port unset
// so that the caller applies the SSH default rather than saving a broken value.
func parsePort(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < domain.MinPort || n > domain.MaxPort {
		return 0
	}
	return n
}

func firstArg(d directive) string {
	if len(d.args) == 0 {
		return ""
	}
	return d.args[0]
}

func setFirst(dst *string, value string) {
	if *dst == "" && value != "" {
		*dst = value
	}
}
