package auditlog

import (
	"strings"
	"unicode/utf8"
)

// CommandLineTracker reconstructs the shell command line from PTY keystrokes.
// It logs only the final line content when Enter is pressed.
type CommandLineTracker struct {
	line   []rune
	cursor int
}

// NewCommandLineTracker creates an empty command line tracker.
func NewCommandLineTracker() *CommandLineTracker {
	return &CommandLineTracker{}
}

// Feed processes terminal input bytes. Returns a completed command line when Enter is pressed.
func (t *CommandLineTracker) Feed(data string) (submitted string, ok bool) {
	for len(data) > 0 {
		if data[0] == '\x1b' {
			consumed, handled := t.feedEscape(data)
			data = data[consumed:]
			if handled {
				continue
			}
			if consumed > 0 {
				continue
			}
			_, size := utf8.DecodeRuneInString(data)
			if size <= 0 {
				size = 1
			}
			data = data[size:]
			continue
		}

		r, size := utf8.DecodeRuneInString(data)
		if size <= 0 {
			break
		}
		data = data[size:]

		switch r {
		case '\r', '\n':
			submitted = strings.TrimRight(string(t.line), " ")
			t.reset()
			return submitted, true
		case '\x7f', '\b':
			t.backspace()
		case '\x03', '\x04': // Ctrl+C, Ctrl+D
			t.reset()
		case '\t':
			// ignore tab for command reconstruction
		default:
			if r >= 0x20 && r != 0x7f {
				t.insert(r)
			}
		}
	}
	return "", false
}

func (t *CommandLineTracker) reset() {
	t.line = t.line[:0]
	t.cursor = 0
}

func (t *CommandLineTracker) insert(r rune) {
	if t.cursor >= len(t.line) {
		t.line = append(t.line, r)
	} else {
		t.line = append(t.line[:t.cursor+1], t.line[t.cursor:]...)
		t.line[t.cursor] = r
	}
	t.cursor++
}

func (t *CommandLineTracker) backspace() {
	if t.cursor <= 0 || len(t.line) == 0 {
		return
	}
	t.line = append(t.line[:t.cursor-1], t.line[t.cursor:]...)
	t.cursor--
}

func (t *CommandLineTracker) deleteForward() {
	if t.cursor >= len(t.line) {
		return
	}
	t.line = append(t.line[:t.cursor], t.line[t.cursor+1:]...)
}

func (t *CommandLineTracker) moveHome() {
	t.cursor = 0
}

func (t *CommandLineTracker) moveEnd() {
	t.cursor = len(t.line)
}

func (t *CommandLineTracker) moveLeft(n int) {
	if n <= 0 {
		n = 1
	}
	if t.cursor > n {
		t.cursor -= n
	} else {
		t.cursor = 0
	}
}

func (t *CommandLineTracker) moveRight(n int) {
	if n <= 0 {
		n = 1
	}
	if t.cursor+n > len(t.line) {
		t.cursor = len(t.line)
	} else {
		t.cursor += n
	}
}

func (t *CommandLineTracker) feedEscape(data string) (consumed int, handled bool) {
	if len(data) < 1 || data[0] != '\x1b' {
		return 0, false
	}
	if len(data) == 1 {
		return 1, true
	}

	switch data[1] {
	case '[':
		if len(data) < 3 {
			return len(data), true
		}
		// CSI sequences: ESC [ ... letter
		i := 2
		for i < len(data) {
			c := data[i]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				t.handleCSI(data[2:i], c)
				return i + 1, true
			}
			i++
		}
		return len(data), true
	case 'O':
		if len(data) < 3 {
			return len(data), true
		}
		switch data[2] {
		case 'H':
			t.moveHome()
		case 'F':
			t.moveEnd()
		}
		return 3, true
	default:
		return 2, true
	}
}

func (t *CommandLineTracker) handleCSI(params string, cmd byte) {
	switch cmd {
	case 'A':
		t.moveLeft(1)
	case 'B':
		t.moveRight(1)
	case 'C':
		t.moveRight(1)
	case 'D':
		t.moveLeft(1)
	case 'H', 'F':
		if params == "" || params == "1" {
			if cmd == 'H' {
				t.moveHome()
			} else {
				t.moveEnd()
			}
		}
	case '~':
		if params == "3" {
			t.deleteForward()
		}
	}
}

// Line returns the current in-progress line (for tests).
func (t *CommandLineTracker) Line() string {
	return string(t.line)
}
