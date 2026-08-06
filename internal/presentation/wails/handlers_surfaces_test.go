package wails

import (
	"encoding/base64"
	"strings"
	"testing"
)

type recordingSurfaceCommands struct {
	closed  []string
	inputs  []string
	resizes []string
}

func (r *recordingSurfaceCommands) CloseSurfaceFromUI(surfaceID string) {
	r.closed = append(r.closed, surfaceID)
}

func (r *recordingSurfaceCommands) DeliverInput(surfaceID string, data []byte) {
	r.inputs = append(r.inputs, surfaceID+":"+string(data))
}

func (r *recordingSurfaceCommands) DeliverResize(surfaceID string, cols, rows uint16) {
	r.resizes = append(r.resizes, surfaceID)
}

func apiWithSurfaces(t *testing.T) (*AppAPI, *recordingSurfaceCommands) {
	t.Helper()
	rec := &recordingSurfaceCommands{}
	api := &AppAPI{}
	api.SetSurfaceService(rec)
	return api, rec
}

// The frontend is not a trust boundary. A payload that is not base64 is refused before it reaches
// the service, so a decode failure never becomes bytes on a remote process's stdin.
func TestSendSurfaceInputRejectsNonBase64(t *testing.T) {
	api, rec := apiWithSurfaces(t)
	if err := api.SendSurfaceInput("srf-1", "not base64!!"); err == nil {
		t.Fatal("expected a non-base64 payload to be refused")
	}
	if len(rec.inputs) != 0 {
		t.Fatal("a refused payload must not reach the service")
	}
}

func TestSendSurfaceInputDeliversDecodedBytes(t *testing.T) {
	api, rec := apiWithSurfaces(t)
	payload := base64.StdEncoding.EncodeToString([]byte("ls\r"))
	if err := api.SendSurfaceInput("srf-1", payload); err != nil {
		t.Fatalf("SendSurfaceInput: %v", err)
	}
	if len(rec.inputs) != 1 || rec.inputs[0] != "srf-1:ls\r" {
		t.Fatalf("inputs = %v", rec.inputs)
	}
}

func TestSendSurfaceInputRejectsEmptySurfaceID(t *testing.T) {
	api, rec := apiWithSurfaces(t)
	if err := api.SendSurfaceInput("  ", base64.StdEncoding.EncodeToString([]byte("x"))); err == nil {
		t.Fatal("expected an empty surfaceId to be refused")
	}
	if len(rec.inputs) != 0 {
		t.Fatal("a refused call must not reach the service")
	}
}

// A geometry the terminal cannot have is refused here rather than forwarded: cols/rows cross the
// boundary into a remote pty, and a zero or absurd value there is a resize nobody asked for.
func TestResizeSurfaceRejectsOutOfRangeDimensions(t *testing.T) {
	api, rec := apiWithSurfaces(t)
	cases := [][2]int{{0, 24}, {80, 0}, {-1, 24}, {80, -1}, {100000, 24}, {80, 100000}}
	for _, c := range cases {
		if err := api.ResizeSurface("srf-1", c[0], c[1]); err == nil {
			t.Fatalf("expected cols=%d rows=%d to be refused", c[0], c[1])
		}
	}
	if len(rec.resizes) != 0 {
		t.Fatal("a refused resize must not reach the service")
	}
}

func TestResizeSurfaceForwardsValidDimensions(t *testing.T) {
	api, rec := apiWithSurfaces(t)
	if err := api.ResizeSurface("srf-1", 120, 40); err != nil {
		t.Fatalf("ResizeSurface: %v", err)
	}
	if len(rec.resizes) != 1 {
		t.Fatalf("resizes = %v", rec.resizes)
	}
}

func TestCloseSurfaceForwards(t *testing.T) {
	api, rec := apiWithSurfaces(t)
	if err := api.CloseSurface("srf-1"); err != nil {
		t.Fatalf("CloseSurface: %v", err)
	}
	if len(rec.closed) != 1 || rec.closed[0] != "srf-1" {
		t.Fatalf("closed = %v", rec.closed)
	}
}

// Without a service wired, the handlers report a plain error rather than panicking: the vault may
// be locked, or the plugin runtime may not have been composed at all.
func TestSurfaceHandlersAreInertWithoutService(t *testing.T) {
	api := &AppAPI{}
	if err := api.CloseSurface("srf-1"); err == nil {
		t.Fatal("CloseSurface must report unavailability")
	}
	if err := api.SendSurfaceInput("srf-1", ""); err == nil {
		t.Fatal("SendSurfaceInput must report unavailability")
	}
	if err := api.ResizeSurface("srf-1", 80, 24); err == nil {
		t.Fatal("ResizeSurface must report unavailability")
	}
}

// The error a handler returns is shown to a user and must not carry a plugin's own text or an id
// that reads like internal state.
func TestSurfaceHandlerErrorsStayGeneric(t *testing.T) {
	api := &AppAPI{}
	err := api.CloseSurface("srf-secret-id")
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "srf-secret-id") {
		t.Fatalf("handler error leaks the surface id: %v", err)
	}
}
