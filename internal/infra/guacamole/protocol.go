package guacamole

import (
	"fmt"
	"strconv"
	"strings"
)

// FormatInstruction encodes elements into a Guacamole protocol instruction.
// The first element is the opcode; remaining elements are arguments.
// Example: FormatInstruction("select", "rdp") → "6.select,3.rdp;"
func FormatInstruction(elements ...string) string {
	var b strings.Builder
	for i, el := range elements {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(len(el)))
		b.WriteByte('.')
		b.WriteString(el)
	}
	b.WriteByte(';')
	return b.String()
}

// ParseInstruction parses the first complete instruction from data.
// Returns opcode, args, number of bytes consumed, and error.
// If data doesn't contain a complete instruction, returns ("", nil, 0, nil).
func ParseInstruction(data string) (opcode string, args []string, consumed int, err error) {
	pos := 0
	var elements []string

	for pos < len(data) {
		dotIdx := strings.IndexByte(data[pos:], '.')
		if dotIdx < 0 {
			return "", nil, 0, nil
		}

		lenStr := data[pos : pos+dotIdx]
		length, parseErr := strconv.Atoi(lenStr)
		if parseErr != nil {
			return "", nil, 0, fmt.Errorf("invalid element length %q: %w", lenStr, parseErr)
		}
		if length < 0 {
			return "", nil, 0, fmt.Errorf("negative element length %d", length)
		}

		valueStart := pos + dotIdx + 1
		valueEnd := valueStart + length
		if valueEnd > len(data) {
			return "", nil, 0, nil
		}

		elements = append(elements, data[valueStart:valueEnd])
		pos = valueEnd

		if pos >= len(data) {
			return "", nil, 0, nil
		}

		terminator := data[pos]
		pos++

		if terminator == ';' {
			if len(elements) == 0 {
				return "", nil, 0, fmt.Errorf("empty instruction")
			}
			return elements[0], elements[1:], pos, nil
		}
		if terminator != ',' {
			return "", nil, 0, fmt.Errorf("unexpected character %q after element", terminator)
		}
	}

	return "", nil, 0, nil
}

// InstructionParser accumulates data and yields complete Guacamole instructions.
type InstructionParser struct {
	buf string
}

// Feed adds raw data to the parser buffer.
func (p *InstructionParser) Feed(data string) {
	p.buf += data
}

// Next returns the next complete instruction from the buffer.
// Returns ("", nil, nil) when no complete instruction is available.
func (p *InstructionParser) Next() (opcode string, args []string, err error) {
	opcode, args, consumed, err := ParseInstruction(p.buf)
	if err != nil {
		return "", nil, err
	}
	if consumed == 0 {
		return "", nil, nil
	}
	p.buf = p.buf[consumed:]
	return opcode, args, nil
}

// Remaining returns unprocessed data left in the buffer.
func (p *InstructionParser) Remaining() string {
	return p.buf
}
