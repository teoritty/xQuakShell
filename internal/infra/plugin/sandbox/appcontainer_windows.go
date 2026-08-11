//go:build windows

package sandbox

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The AppContainer profile calls live in userenv.dll and x/sys does not wrap them, so they are
// bound by hand. All three return an HRESULT rather than setting the last error, which is why none
// of them uses the errno the Call returns.
var (
	userenv                    = windows.NewLazySystemDLL("userenv.dll")
	procCreateAppContainer     = userenv.NewProc("CreateAppContainerProfile")
	procDeleteAppContainer     = userenv.NewProc("DeleteAppContainerProfile")
	procDeriveAppContainerSID  = userenv.NewProc("DeriveAppContainerSidFromAppContainerName")
	errProfileAPIsUnavailable  = fmt.Errorf("userenv.dll does not export the AppContainer profile calls")
	appContainerProfileMissing = uint32(0x80070002) // HRESULT_FROM_WIN32(ERROR_FILE_NOT_FOUND)
)

const appContainerProfileExists = uint32(0x800700B7) // HRESULT_FROM_WIN32(ERROR_ALREADY_EXISTS)

// Container is one AppContainer profile and the SID that identifies it.
//
// The SID is the whole point: it is what an ACL names, and it is derived from the profile name, so
// it is stable across restarts. That stability is what lets the ACEs written at install time still
// mean something at the next start, and what makes a profile left behind by a crash safe to reuse
// rather than something to work around.
type Container struct {
	Name string
	sid  *windows.SID
}

// SID returns the container's identity, for the ACL and for the spawn.
func (c *Container) SID() *windows.SID { return c.sid }

// String renders the SID, for the log and the audit line.
func (c *Container) String() string {
	if c == nil || c.sid == nil {
		return ""
	}
	return c.sid.String()
}

// ProfileAPIAvailable reports whether this Windows build exposes the AppContainer profile calls.
//
// It is a lookup and not a probe: creating a throwaway profile to find out would write to the
// user's registry on every application start, and the honest answer to "did it work" belongs to the
// attempt that actually matters, not to a rehearsal of it.
func ProfileAPIAvailable() bool {
	for _, proc := range []*windows.LazyProc{
		procCreateAppContainer, procDeleteAppContainer, procDeriveAppContainerSID,
	} {
		if err := proc.Find(); err != nil {
			return false
		}
	}
	return true
}

// CreateContainer creates the profile for name, or adopts the one already there.
//
// An existing profile is not a conflict. The name is a hash of the plugin instance's identity
// (see ContainerName), so a profile under that name describes this same instance and nobody else —
// it is what a crash or a power loss left behind, and its SID is the one this instance's ACEs
// already name. Refusing it would leave a plugin permanently unable to start until something swept
// the profile away.
func CreateContainer(name, displayName, description string) (*Container, error) {
	if !ProfileAPIAvailable() {
		return nil, errProfileAPIsUnavailable
	}
	namePtr, displayPtr, descPtr, err := profileStrings(name, displayName, description)
	if err != nil {
		return nil, err
	}

	// Twice, because a profile is shared mutable state in the user's registry and another copy of
	// this application sweeping orphans at ITS startup can delete one between the two halves of
	// this call. Both halves are idempotent — creating adopts, adopting recreates — so a second
	// attempt resolves the only interleaving that can fail, and a second failure is a real one.
	container, err := createOrOpenContainer(name, namePtr, displayPtr, descPtr)
	if err != nil {
		return createOrOpenContainer(name, namePtr, displayPtr, descPtr)
	}
	return container, nil
}

func createOrOpenContainer(name string, namePtr, displayPtr, descPtr *uint16) (*Container, error) {
	var raw *windows.SID
	hr, _, _ := procCreateAppContainer.Call(
		uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(displayPtr)),
		uintptr(unsafe.Pointer(descPtr)), 0, 0, uintptr(unsafe.Pointer(&raw)))
	switch hresult(hr) {
	case 0:
		defer func() { _ = windows.FreeSid(raw) }()
		return containerFromSID(name, raw)
	case appContainerProfileExists:
		return OpenContainer(name)
	default:
		return nil, hresultError("CreateAppContainerProfile", hresult(hr))
	}
}

// OpenContainer derives the SID of a profile without creating one. The derivation is pure — the
// same name always yields the same SID, whether or not a profile exists — so this is also how the
// teardown paths address a profile they are about to delete.
func OpenContainer(name string) (*Container, error) {
	if !ProfileAPIAvailable() {
		return nil, errProfileAPIsUnavailable
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("app container name: %w", err)
	}
	var raw *windows.SID
	hr, _, _ := procDeriveAppContainerSID.Call(
		uintptr(unsafe.Pointer(namePtr)), uintptr(unsafe.Pointer(&raw)))
	if hresult(hr) != 0 {
		return nil, hresultError("DeriveAppContainerSidFromAppContainerName", hresult(hr))
	}
	defer func() { _ = windows.FreeSid(raw) }()
	return containerFromSID(name, raw)
}

// DeleteContainer removes the profile, its registry mapping and its directory under Packages.
//
// A profile that is not there is success, not an error. Every caller is a teardown path and they
// overlap on purpose — a session closing, a plugin being uninstalled, and the orphan sweep at
// startup can all reach the same profile, and the second one to arrive must not report a failure
// for finding the work already done.
func DeleteContainer(name string) error {
	if !ProfileAPIAvailable() {
		return errProfileAPIsUnavailable
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return fmt.Errorf("app container name: %w", err)
	}
	hr, _, _ := procDeleteAppContainer.Call(uintptr(unsafe.Pointer(namePtr)))
	switch hresult(hr) {
	case 0, appContainerProfileMissing:
		return nil
	default:
		return hresultError("DeleteAppContainerProfile", hresult(hr))
	}
}

func containerFromSID(name string, raw *windows.SID) (*Container, error) {
	// The call returns a SID on a heap this package does not own and must free. Copying it onto the
	// Go heap first means the Container can outlive that free without anyone tracking two
	// lifetimes.
	sid, err := raw.Copy()
	if err != nil {
		return nil, fmt.Errorf("copy app container sid: %w", err)
	}
	return &Container{Name: name, sid: sid}, nil
}

func profileStrings(name, displayName, description string) (n, d, desc *uint16, err error) {
	if n, err = windows.UTF16PtrFromString(name); err != nil {
		return nil, nil, nil, fmt.Errorf("app container name: %w", err)
	}
	if d, err = windows.UTF16PtrFromString(displayName); err != nil {
		return nil, nil, nil, fmt.Errorf("app container display name: %w", err)
	}
	if desc, err = windows.UTF16PtrFromString(description); err != nil {
		return nil, nil, nil, fmt.Errorf("app container description: %w", err)
	}
	return n, d, desc, nil
}

// hresult narrows the value a userenv call returned into what it actually is.
//
// The three profile calls return an HRESULT, which is 32 bits by definition; syscall.Call hands it
// back in a uintptr because that is the width of a register. Nothing is lost — the upper half is
// never set — and the alternative to the conversion is carrying a 64-bit value that cannot be
// compared against any documented HRESULT constant.
//
// #nosec G115 -- an HRESULT is a 32-bit value in a register-sized return; see above.
func hresult(r uintptr) uint32 {
	return uint32(r)
}

// hresultError unwraps the Win32 error an HRESULT was built from, so the message says "access is
// denied" rather than "0x80070005" — and so errors.Is against a syscall.Errno still works.
func hresultError(call string, hr uint32) error {
	if hr&0xFFFF0000 == 0x80070000 {
		return fmt.Errorf("%s: %w", call, syscall.Errno(hr&0xFFFF))
	}
	return fmt.Errorf("%s: HRESULT 0x%08x", call, hr)
}
