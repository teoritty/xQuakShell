// Package sshconfig parses OpenSSH client configuration files into the
// importable host descriptions defined by the domain layer.
//
// The parser deliberately implements only the subset of ssh_config(5) that can
// be evaluated statically, without opening a connection: Host blocks, Include,
// HostName, User, Port, IdentityFile and ProxyJump. Everything else is either
// ignored or surfaced as a notice, so the user is told what was not honoured
// rather than silently receiving an incomplete import.
package sshconfig

import "strings"

// directive is one meaningful configuration line: a keyword and its arguments.
type directive struct {
	// keyword is lower-cased; ssh_config keywords are case-insensitive.
	keyword string
	args    []string
	// line is the 1-based line number within its own file, used for stable
	// ordering only — it is never shown to the user.
	line int
}

// lexLine splits one raw configuration line into a directive.
//
// It implements the ssh_config(5) surface syntax: comments introduced by '#',
// a keyword separated from its arguments by whitespace and/or a single '=',
// and double-quoted arguments that may contain spaces. It returns ok=false for
// blank and comment-only lines.
func lexLine(raw string) (directive, bool) {
	line := strings.TrimSpace(stripComment(raw))
	if line == "" {
		return directive{}, false
	}
	// The keyword may be separated from the first argument by whitespace, '=',
	// or both ("Port=22", "Port = 22", "Port 22" are equivalent).
	keyword, rest := splitKeyword(line)
	if keyword == "" {
		return directive{}, false
	}
	return directive{keyword: strings.ToLower(keyword), args: splitArgs(rest)}, true
}

// stripComment removes a trailing comment, honouring double quotes so that a
// '#' inside a quoted argument is kept as data.
func stripComment(raw string) string {
	inQuotes := false
	for i, r := range raw {
		switch r {
		case '"':
			inQuotes = !inQuotes
		case '#':
			if !inQuotes {
				return raw[:i]
			}
		}
	}
	return raw
}

// splitKeyword separates the leading keyword from the remainder of the line.
func splitKeyword(line string) (string, string) {
	idx := strings.IndexFunc(line, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '='
	})
	if idx < 0 {
		return line, ""
	}
	keyword := line[:idx]
	rest := strings.TrimLeft(line[idx:], " \t")
	// A single '=' may follow the whitespace; further '=' are argument data.
	rest = strings.TrimLeft(strings.TrimPrefix(rest, "="), " \t")
	return keyword, rest
}

// splitArgs splits an argument list on whitespace, keeping double-quoted runs
// together and dropping the quotes themselves.
func splitArgs(rest string) []string {
	var (
		args     []string
		current  strings.Builder
		inQuotes bool
		started  bool
	)
	flush := func() {
		if started {
			args = append(args, current.String())
			current.Reset()
			started = false
		}
	}
	for _, r := range rest {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			// An empty quoted string is still an argument.
			started = true
		case (r == ' ' || r == '\t') && !inQuotes:
			flush()
		default:
			current.WriteRune(r)
			started = true
		}
	}
	flush()
	return args
}

// lexFile turns a whole file into directives, skipping blanks and comments.
// A UTF-8 BOM and CRLF line endings are tolerated: config files copied from
// Windows editors routinely carry both.
func lexFile(content string) []directive {
	content = strings.TrimPrefix(content, "\ufeff")
	rawLines := strings.Split(content, "\n")
	directives := make([]directive, 0, len(rawLines))
	for i, raw := range rawLines {
		d, ok := lexLine(strings.TrimSuffix(raw, "\r"))
		if !ok {
			continue
		}
		d.line = i + 1
		directives = append(directives, d)
	}
	return directives
}
