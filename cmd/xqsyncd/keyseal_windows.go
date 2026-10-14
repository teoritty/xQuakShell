//go:build windows

package main

import (
	"bytes"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sealKeyFile protects the key file with DPAPI in machine scope (spec 9.4). Machine scope, not the
// user's: the installer writes the file as an Administrator and the service reads it as its
// virtual account, and both run on this machine. What it buys is that a copy taken off the machine
// - a backup of the data directory - does not carry the keys in the clear; on the machine the
// directory's ACL is what keeps other accounts out.
func sealKeyFile(plain []byte) ([]byte, error) {
	in := blobOf(plain)
	var out windows.DataBlob
	err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN|windows.CRYPTPROTECT_LOCAL_MACHINE, &out)
	if err != nil {
		return nil, err
	}
	return takeBlob(&out), nil
}

// openKeyFile unseals a DPAPI key file. A plain JSON file - one written on Linux and carried over -
// is read as it is, so moving a server to Windows does not lock its accounts out.
func openKeyFile(raw []byte) ([]byte, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '{' {
		return raw, nil
	}
	in := blobOf(raw)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	return takeBlob(&out), nil
}

func blobOf(b []byte) windows.DataBlob {
	if len(b) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]} // #nosec G115 -- a key file is a few hundred bytes
}

// takeBlob copies DPAPI's output into Go memory and frees the system allocation.
func takeBlob(b *windows.DataBlob) []byte {
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(b.Data))) }() // #nosec G103 -- freeing what DPAPI allocated
	return bytes.Clone(unsafe.Slice(b.Data, b.Size))
}
