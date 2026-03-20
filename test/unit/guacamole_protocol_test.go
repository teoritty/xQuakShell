package unit

import (
	"testing"

	"ssh-client/internal/infra/guacamole"
)

func TestFormatInstruction(t *testing.T) {
	tests := []struct {
		name     string
		elements []string
		want     string
	}{
		{
			name:     "select rdp",
			elements: []string{"select", "rdp"},
			want:     "6.select,3.rdp;",
		},
		{
			name:     "single element",
			elements: []string{"nop"},
			want:     "3.nop;",
		},
		{
			name:     "empty opcode with UUID (internal instruction)",
			elements: []string{"", "abc-123-uuid"},
			want:     "0.,12.abc-123-uuid;",
		},
		{
			name:     "size instruction",
			elements: []string{"size", "1920", "1080", "96"},
			want:     "4.size,4.1920,4.1080,2.96;",
		},
		{
			name:     "connect with args",
			elements: []string{"connect", "VERSION_1_1_0", "myhost", "3389", "admin", "secret", "", "any", "true"},
			want:     "7.connect,13.VERSION_1_1_0,6.myhost,4.3389,5.admin,6.secret,0.,3.any,4.true;",
		},
		{
			name:     "audio with no codecs",
			elements: []string{"audio"},
			want:     "5.audio;",
		},
		{
			name:     "image with codecs",
			elements: []string{"image", "image/png", "image/jpeg"},
			want:     "5.image,9.image/png,10.image/jpeg;",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := guacamole.FormatInstruction(tt.elements...)
			if got != tt.want {
				t.Errorf("FormatInstruction(%v) = %q, want %q", tt.elements, got, tt.want)
			}
		})
	}
}

func TestParseInstruction(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		opcode   string
		args     []string
		consumed int
		wantErr  bool
	}{
		{
			name:     "simple select",
			data:     "6.select,3.rdp;",
			opcode:   "select",
			args:     []string{"rdp"},
			consumed: 15,
		},
		{
			name:     "nop",
			data:     "3.nop;",
			opcode:   "nop",
			args:     []string{},
			consumed: 6,
		},
		{
			name:     "internal instruction (empty opcode)",
			data:     "0.,12.abc-123-uuid;",
			opcode:   "",
			args:     []string{"abc-123-uuid"},
			consumed: 19,
		},
		{
			name:     "incomplete data",
			data:     "6.select,3.rd",
			opcode:   "",
			consumed: 0,
		},
		{
			name:     "incomplete length",
			data:     "6.select",
			opcode:   "",
			consumed: 0,
		},
		{
			name:     "two instructions concatenated",
			data:     "3.nop;6.select,3.rdp;",
			opcode:   "nop",
			args:     []string{},
			consumed: 6,
		},
		{
			name:    "invalid length",
			data:    "abc.test;",
			wantErr: true,
		},
		{
			name:    "bad terminator",
			data:    "3.nop!",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opcode, args, consumed, err := guacamole.ParseInstruction(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if opcode != tt.opcode {
				t.Errorf("opcode = %q, want %q", opcode, tt.opcode)
			}
			if consumed != tt.consumed {
				t.Errorf("consumed = %d, want %d", consumed, tt.consumed)
			}
			if args == nil {
				args = []string{}
			}
			if tt.args == nil {
				tt.args = []string{}
			}
			if len(args) != len(tt.args) {
				t.Errorf("args len = %d, want %d", len(args), len(tt.args))
			} else {
				for i := range args {
					if args[i] != tt.args[i] {
						t.Errorf("args[%d] = %q, want %q", i, args[i], tt.args[i])
					}
				}
			}
		})
	}
}

func TestInstructionParserMultiple(t *testing.T) {
	p := &guacamole.InstructionParser{}
	p.Feed("3.nop;6.select,3.rdp;")

	opcode1, args1, err := p.Next()
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if opcode1 != "nop" {
		t.Errorf("first opcode = %q, want nop", opcode1)
	}
	if len(args1) != 0 {
		t.Errorf("first args = %v, want empty", args1)
	}

	opcode2, args2, err := p.Next()
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if opcode2 != "select" {
		t.Errorf("second opcode = %q, want select", opcode2)
	}
	if len(args2) != 1 || args2[0] != "rdp" {
		t.Errorf("second args = %v, want [rdp]", args2)
	}

	opcode3, _, err := p.Next()
	if err != nil {
		t.Fatalf("third Next: %v", err)
	}
	if opcode3 != "" {
		t.Errorf("third opcode = %q, want empty (no more instructions)", opcode3)
	}
}

func TestInstructionParserChunked(t *testing.T) {
	p := &guacamole.InstructionParser{}

	p.Feed("6.sele")
	opcode, _, err := p.Next()
	if err != nil {
		t.Fatalf("chunk1: %v", err)
	}
	if opcode != "" {
		t.Fatalf("chunk1: expected empty, got %q", opcode)
	}

	p.Feed("ct,3.rdp;")
	opcode, args, err := p.Next()
	if err != nil {
		t.Fatalf("chunk2: %v", err)
	}
	if opcode != "select" {
		t.Fatalf("chunk2: opcode = %q, want select", opcode)
	}
	if len(args) != 1 || args[0] != "rdp" {
		t.Fatalf("chunk2: args = %v, want [rdp]", args)
	}
}

func TestFormatThenParse(t *testing.T) {
	original := []string{"connect", "VERSION_1_1_0", "192.168.1.100", "3389", "admin", "p@ss", "", "any", "true"}
	formatted := guacamole.FormatInstruction(original...)

	opcode, args, consumed, err := guacamole.ParseInstruction(formatted)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if consumed != len(formatted) {
		t.Fatalf("consumed = %d, want %d", consumed, len(formatted))
	}
	if opcode != "connect" {
		t.Fatalf("opcode = %q, want connect", opcode)
	}

	expected := original[1:]
	if len(args) != len(expected) {
		t.Fatalf("args length = %d, want %d", len(args), len(expected))
	}
	for i, a := range args {
		if a != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, a, expected[i])
		}
	}
}
