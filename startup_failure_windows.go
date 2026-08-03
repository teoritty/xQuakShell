//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// A native message box, called directly rather than through a dependency: this runs when the app
// has failed to start, so it must not need anything the failure could have taken with it.
//
// MB_OK | MB_ICONERROR | MB_SETFOREGROUND | MB_TOPMOST — the last two because the process has no
// window of its own to own the dialog, and without them it can open behind everything else, which
// is indistinguishable from the silence it exists to replace.
const (
	mbOK            = 0x00000000
	mbIconError     = 0x00000010
	mbSetForeground = 0x00010000
	mbTopMost       = 0x00040000
)

func displayStartupFailure(title, body string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	messageBoxW := user32.NewProc("MessageBoxW")

	titleUTF16, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	bodyUTF16, err := syscall.UTF16PtrFromString(body)
	if err != nil {
		return
	}

	_, _, _ = messageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(bodyUTF16)),
		uintptr(unsafe.Pointer(titleUTF16)),
		uintptr(mbOK|mbIconError|mbSetForeground|mbTopMost),
	)
}
