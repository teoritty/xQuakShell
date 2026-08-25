package wails

import (
	"errors"
	"reflect"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

type fakeLocalTerminals struct {
	opened  int
	writes  map[string][]byte
	resizes map[string][2]uint16
	closed  []string
	shells  []domain.ShellOption
	openErr error
}

func newFakeLocalTerminals() *fakeLocalTerminals {
	return &fakeLocalTerminals{writes: map[string][]byte{}, resizes: map[string][2]uint16{}}
}

func (f *fakeLocalTerminals) Open() (usecase.LocalTerminalInfo, error) {
	if f.openErr != nil {
		return usecase.LocalTerminalInfo{}, f.openErr
	}
	f.opened++
	return usecase.LocalTerminalInfo{ID: "lt-1", Title: "bash"}, nil
}

func (f *fakeLocalTerminals) Write(id string, data []byte) error {
	f.writes[id] = append(f.writes[id], data...)
	return nil
}

func (f *fakeLocalTerminals) Resize(id string, cols, rows uint16) error {
	f.resizes[id] = [2]uint16{cols, rows}
	return nil
}

func (f *fakeLocalTerminals) Close(id string) error {
	f.closed = append(f.closed, id)
	return nil
}

func (f *fakeLocalTerminals) CloseAll() { f.closed = append(f.closed, "*") }

func (f *fakeLocalTerminals) ListShells() []domain.ShellOption { return f.shells }

var _ LocalTerminalCommands = (*fakeLocalTerminals)(nil)

func apiWithLocalTerminals(svc LocalTerminalCommands) *AppAPI {
	api := &AppAPI{}
	api.SetLocalTerminalService(svc)
	return api
}

func TestLocalTerminalHandlersRefuseWhenNothingIsWired(t *testing.T) {
	api := &AppAPI{}

	if _, err := api.OpenLocalTerminal(); !errors.Is(err, errLocalTerminalsUnavailable) {
		t.Errorf("OpenLocalTerminal() = %v, want unavailable", err)
	}
	if err := api.CloseLocalTerminal("lt-1"); !errors.Is(err, errLocalTerminalsUnavailable) {
		t.Errorf("CloseLocalTerminal() = %v, want unavailable", err)
	}
	if err := api.SendLocalTerminalInput("lt-1", ""); !errors.Is(err, errLocalTerminalsUnavailable) {
		t.Errorf("SendLocalTerminalInput() = %v, want unavailable", err)
	}
	if err := api.ResizeLocalTerminal("lt-1", 80, 24); !errors.Is(err, errLocalTerminalsUnavailable) {
		t.Errorf("ResizeLocalTerminal() = %v, want unavailable", err)
	}
	if _, err := api.ListLocalShells(); !errors.Is(err, errLocalTerminalsUnavailable) {
		t.Errorf("ListLocalShells() = %v, want unavailable", err)
	}
}

func TestOpenLocalTerminalReturnsTheTab(t *testing.T) {
	fake := newFakeLocalTerminals()
	dto, err := apiWithLocalTerminals(fake).OpenLocalTerminal()
	if err != nil {
		t.Fatalf("OpenLocalTerminal() = %v, want nil", err)
	}
	if dto.ID != "lt-1" || dto.Title != "bash" {
		t.Errorf("dto = %+v, want the id and title the use case returned", dto)
	}
}

func TestSendLocalTerminalInputDecodesBase64(t *testing.T) {
	fake := newFakeLocalTerminals()
	api := apiWithLocalTerminals(fake)

	// "ls\r" - keystrokes travel base64 because they are bytes, not text.
	if err := api.SendLocalTerminalInput("lt-1", "bHMN"); err != nil {
		t.Fatalf("SendLocalTerminalInput() = %v, want nil", err)
	}
	if got := string(fake.writes["lt-1"]); got != "ls\r" {
		t.Errorf("decoded %q, want \"ls\\r\"", got)
	}
}

func TestLocalTerminalHandlersRejectBadArguments(t *testing.T) {
	api := apiWithLocalTerminals(newFakeLocalTerminals())

	cases := []struct {
		name string
		err  error
	}{
		{"empty id on close", api.CloseLocalTerminal("")},
		{"empty id on input", api.SendLocalTerminalInput("", "bHM=")},
		{"invalid base64", api.SendLocalTerminalInput("lt-1", "not base64!!")},
		{"empty id on resize", api.ResizeLocalTerminal("", 80, 24)},
		{"zero cols", api.ResizeLocalTerminal("lt-1", 0, 24)},
		{"negative rows", api.ResizeLocalTerminal("lt-1", 80, -1)},
		{"absurd cols", api.ResizeLocalTerminal("lt-1", maxSurfaceDimension+1, 24)},
	}
	for _, tc := range cases {
		if !errors.Is(tc.err, errInvalidLocalTerminalRequest) {
			t.Errorf("%s: err = %v, want the generic rejection", tc.name, tc.err)
		}
	}
}

func TestResizeLocalTerminalPassesTheGridThrough(t *testing.T) {
	fake := newFakeLocalTerminals()
	if err := apiWithLocalTerminals(fake).ResizeLocalTerminal("lt-1", 120, 40); err != nil {
		t.Fatalf("ResizeLocalTerminal() = %v, want nil", err)
	}
	if got := fake.resizes["lt-1"]; got != [2]uint16{120, 40} {
		t.Errorf("resize = %v, want 120x40", got)
	}
}

// TestOpenLocalTerminalTakesNoArguments is the security invariant as a test.
//
// The whole design rests on the frontend being unable to influence which program runs. A
// parameter added here - a path, a command, "just the arguments" - would turn a feature with no
// injection surface into one with the usual one, and would do so in a diff that otherwise looks
// like a small convenience.
func TestOpenLocalTerminalTakesNoArguments(t *testing.T) {
	method, ok := reflect.TypeOf(&AppAPI{}).MethodByName("OpenLocalTerminal")
	if !ok {
		t.Fatal("OpenLocalTerminal is missing")
	}
	// The receiver is the only parameter a niladic method has.
	if got := method.Type.NumIn(); got != 1 {
		t.Errorf("OpenLocalTerminal takes %d arguments, want none: nothing from the frontend may "+
			"influence which program is executed", got-1)
	}
}
