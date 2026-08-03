package sshconfig

import "strings"

// block is one `Host` (or `Match`) section: the patterns it applies to and the
// directives it contributes, in file order.
type block struct {
	patterns []pattern
	settings []directive
	// isMatch marks a `Match` block. Match conditions depend on runtime state
	// (the resolved hostname, the result of an exec test, whether this is the
	// final pass), so the importer records the block's existence and skips it
	// rather than guessing which hosts it would apply to.
	isMatch bool
}

// pattern is a single Host pattern, e.g. `web-*` or `!web-staging`.
type pattern struct {
	text     string
	negated  bool
	wildcard bool
}

// newPattern classifies one raw Host pattern token.
func newPattern(raw string) pattern {
	p := pattern{text: raw}
	if strings.HasPrefix(raw, "!") {
		p.negated = true
		p.text = raw[1:]
	}
	p.wildcard = strings.ContainsAny(p.text, "*?")
	return p
}

// groupBlocks turns a flat directive stream into ordered blocks.
//
// Directives appearing before any Host/Match keyword apply globally in
// OpenSSH; they are modelled as a leading block matching everything, which
// makes them behave exactly like a `Host *` block during resolution.
func groupBlocks(directives []directive) []block {
	var (
		blocks  []block
		current = block{patterns: []pattern{{text: "*", wildcard: true}}}
	)
	for _, d := range directives {
		switch d.keyword {
		case "host":
			blocks = append(blocks, current)
			current = block{patterns: toPatterns(d.args)}
		case "match":
			blocks = append(blocks, current)
			current = block{isMatch: true}
		default:
			current.settings = append(current.settings, d)
		}
	}
	return append(blocks, current)
}

func toPatterns(args []string) []pattern {
	patterns := make([]pattern, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			continue
		}
		patterns = append(patterns, newPattern(arg))
	}
	return patterns
}

// matches reports whether the block applies to a concrete host alias, using
// OpenSSH's rule: at least one positive pattern must match and no negated
// pattern may match.
func (b block) matches(alias string) bool {
	if b.isMatch {
		return false
	}
	positive := false
	for _, p := range b.patterns {
		if !matchPattern(p.text, alias) {
			continue
		}
		if p.negated {
			return false
		}
		positive = true
	}
	return positive
}

// isDefaultsOnly reports whether the block can never name an importable host —
// every pattern is a wildcard or a negation. Such blocks only contribute
// settings to hosts declared elsewhere.
func (b block) isDefaultsOnly() bool {
	for _, p := range b.patterns {
		if !p.wildcard && !p.negated {
			return false
		}
	}
	return true
}

// concreteAliases returns the literal host names this block declares, i.e. the
// candidates for becoming connections.
func (b block) concreteAliases() []string {
	var aliases []string
	for _, p := range b.patterns {
		if p.wildcard || p.negated || p.text == "" {
			continue
		}
		aliases = append(aliases, p.text)
	}
	return aliases
}

// matchPattern implements ssh_config's glob subset: '*' matches any run of
// characters and '?' matches exactly one. Matching is case-insensitive, as
// host names are.
func matchPattern(pat, name string) bool {
	return globMatch(strings.ToLower(pat), strings.ToLower(name))
}

// globMatch is an iterative wildcard matcher with backtracking. It is written
// without recursion so that a pathological pattern cannot exhaust the stack.
func globMatch(pat, name string) bool {
	var (
		p, n     int
		starPat  = -1
		starName int
	)
	for n < len(name) {
		switch {
		case p < len(pat) && (pat[p] == '?' || pat[p] == name[n]):
			p++
			n++
		case p < len(pat) && pat[p] == '*':
			starPat, starName = p, n
			p++
		case starPat >= 0:
			// Backtrack: let the last '*' consume one more character.
			starName++
			p, n = starPat+1, starName
		default:
			return false
		}
	}
	for p < len(pat) && pat[p] == '*' {
		p++
	}
	return p == len(pat)
}
