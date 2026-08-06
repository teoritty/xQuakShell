package putty

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// PuTTYSession represents a parsed PuTTY session from a REG file.
type PuTTYSession struct {
	Name     string
	HostName string
	Port     int
	UserName string
	Protocol string
}

// ParsePuTTYReg parses a Windows REG file export of PuTTY sessions.
// Returns sessions that have HostName set (SSH-compatible).
func ParsePuTTYReg(content string) ([]PuTTYSession, error) {
	var sessions []PuTTYSession
	var current *PuTTYSession

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			if current != nil && current.HostName != "" {
				sessions = append(sessions, *current)
			}
			path := line[1 : len(line)-1]
			if idx := strings.LastIndex(path, "\\"); idx >= 0 {
				current = &PuTTYSession{
					Name:     path[idx+1:],
					Port:     22,
					Protocol: "ssh",
				}
			} else {
				current = nil
			}
			continue
		}

		if current == nil {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.Trim(strings.TrimSpace(parts[0]), `"`)
		val := strings.TrimSpace(parts[1])

		switch key {
		case "HostName":
			current.HostName = strings.Trim(val, `"`)
		case "PortNumber":
			if strings.HasPrefix(val, "dword:") {
				hex := val[6:]
				if n, err := strconv.ParseInt(hex, 16, 32); err == nil {
					current.Port = int(n)
				}
			}
		case "UserName":
			current.UserName = strings.Trim(val, `"`)
		case "Protocol":
			current.Protocol = strings.Trim(val, `"`)
		}
	}

	if current != nil && current.HostName != "" {
		sessions = append(sessions, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read reg: %w", err)
	}

	return sessions, nil
}
