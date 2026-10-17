//go:build !windows

package main

// On Linux the key file's protection is its mode and the service's own user (StateDirectoryMode
// 0700, UMask 0077); there is no machine key store to seal it with.
func sealKeyFile(plain []byte) ([]byte, error) { return plain, nil }
func openKeyFile(raw []byte) ([]byte, error)   { return raw, nil }
