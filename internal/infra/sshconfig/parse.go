package sshconfig

import (
	"xquakshell/internal/domain"
)

// Parse reads an OpenSSH client configuration file and resolves it into the
// hosts that can be imported as connections.
//
// Only an unreadable, oversized or missing root file is an error. Everything
// else a real-world config can contain — unknown keywords, Match blocks,
// unreadable includes, dangling IdentityFile paths, unusable ProxyJump
// specifications — is reported as a notice, because a config with one broken
// line should still let the user import the other forty hosts.
func Parse(path string) (domain.SSHConfigParseResult, error) {
	notices := newNoticeSet()
	home := userHomeDir()

	directives, err := newLoader(home, notices).load(path)
	if err != nil {
		return domain.SSHConfigParseResult{}, err
	}

	r := &resolver{blocks: groupBlocks(directives), homeDir: home, notices: notices}
	aliases := r.aliases()
	hosts := make([]domain.SSHConfigHost, 0, len(aliases))
	for _, alias := range aliases {
		hosts = append(hosts, r.buildHost(alias))
	}
	return domain.SSHConfigParseResult{Hosts: hosts, Notices: notices.list()}, nil
}
