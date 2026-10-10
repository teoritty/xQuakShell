package agentforward

import "encoding/binary"

// reader walks SSH wire-format data and remembers the first failure, so a parser can read every
// field in sequence and check once at the end. Every length is checked against what remains
// before anything is sliced: a length prefix is attacker-chosen, and trusting one is how a parser
// reads past its buffer or allocates whatever the peer asked for.
type reader struct {
	buf []byte
	bad bool
}

func (r *reader) byte1() byte {
	if r.bad || len(r.buf) < 1 {
		r.bad = true
		return 0
	}
	b := r.buf[0]
	r.buf = r.buf[1:]
	return b
}

// bool1 accepts only 0 and 1. RFC 4251 lets a decoder read any non-zero byte as true, but a
// strict reader refuses the ambiguity: two encodings of one message means two messages that
// compare unequal while meaning the same thing.
func (r *reader) bool1() bool {
	switch r.byte1() {
	case 0:
		return false
	case 1:
		return true
	default:
		r.bad = true
		return false
	}
}

func (r *reader) str() []byte {
	if r.bad || len(r.buf) < 4 {
		r.bad = true
		return nil
	}
	n := binary.BigEndian.Uint32(r.buf)
	if uint64(n) > uint64(len(r.buf)-4) {
		r.bad = true
		return nil
	}
	s := r.buf[4 : 4+n]
	r.buf = r.buf[4+n:]
	return s
}

// done reports whether every field read cleanly and nothing is left over. Trailing bytes are a
// failure: data the parser did not look at is data that could mean something to whoever reads it
// next, and the signature would cover it.
func (r *reader) done() bool {
	return !r.bad && len(r.buf) == 0
}
