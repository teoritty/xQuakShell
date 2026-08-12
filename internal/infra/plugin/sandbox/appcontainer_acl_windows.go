//go:build windows

package sandbox

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AccessReadExecute and AccessReadWrite are the two grants a plugin instance ever gets: its
// installed files, which it must read and run, and its own data directory, which it must read and
// write. There is deliberately no third.
const (
	AccessReadExecute = windows.GENERIC_READ | windows.GENERIC_EXECUTE
	AccessReadWrite   = windows.GENERIC_READ | windows.GENERIC_WRITE | windows.DELETE
)

// Grant gives the container the rights in mask on path and everything beneath it.
//
// It merges into the existing DACL rather than replacing it: the directory is the user's own, and
// an ACL that named only the container would lock the user out of the plugin they installed.
func (c *Container) Grant(path string, mask uint32) error {
	existing, err := currentDACL(path)
	if err != nil {
		return err
	}
	merged, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.ACCESS_MASK(mask),
		AccessMode:        windows.GRANT_ACCESS,
		// Inheritance matters more than it looks. An AppContainer reads files, not directories, so
		// a grant that stopped at the directory itself would let the plugin list its install tree
		// and open nothing in it.
		Inheritance: windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(c.sid),
		},
	}}, existing)
	if err != nil {
		return fmt.Errorf("build dacl for %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, merged, nil); err != nil {
		return fmt.Errorf("set dacl on %s: %w", path, err)
	}
	return nil
}

// Revoke removes every access this container was granted on path.
//
// It exists because an ACE outlives the profile it was written for. DeleteContainer takes the
// registry entry and the Packages directory; the ACEs stay, and nothing else in this package ever
// removed one — so an identity that is used once and never again leaves a permanent mark on the
// directory it touched. With per-session isolation that identity is a fresh 128-bit session id, so
// the marks accumulate one per connection, on every entry of the install tree, for the life of the
// installation. A DACL is capped at 64 KB; past roughly fifteen hundred of them SetNamedSecurityInfo
// starts refusing, which fails the grant, which fails the start — permanently, and surviving a
// reinstall, because the ACL belongs to the plugin directory rather than to the application.
//
// REVOKE_ACCESS is the whole mechanism: SetEntriesInAcl drops every ACE naming the trustee, so the
// access mask and the inheritance flags are deliberately not repeated here. Getting them wrong
// cannot leave a partial grant behind, because they are not read.
func (c *Container) Revoke(path string) error {
	existing, err := currentDACL(path)
	if err != nil {
		return err
	}
	merged, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessMode: windows.REVOKE_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(c.sid),
		},
	}}, existing)
	if err != nil {
		return fmt.Errorf("build dacl for %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION, nil, nil, merged, nil); err != nil {
		return fmt.Errorf("set dacl on %s: %w", path, err)
	}
	return nil
}

// EnsureGrant applies the grant only when it is not already there.
//
// Verify-then-repair rather than reapply-always, and the difference is measured in plugin start
// latency: SetNamedSecurityInfo on a directory rewrites the ACL of every file under it, so an
// unconditional call is O(files) on every single start of every plugin, to write what is almost
// always already written. The repair path exists for the installs that predate this code and for a
// tree somebody has edited by hand.
func (c *Container) EnsureGrant(path string, mask uint32) error {
	granted, err := c.HasGrant(path, mask)
	if err != nil {
		return err
	}
	if granted {
		return nil
	}
	return c.Grant(path, mask)
}

// HasGrant reports whether the container already holds at least mask on path.
//
// It reads the DACL of the path itself, which also answers for children: Windows materialises an
// inheritable ACE into every child's ACL rather than evaluating it at access time. The child's copy
// is not byte-identical, though — the generic bits are expanded into the specific rights they stand
// for on the way down, so a naive comparison against GENERIC_READ finds nothing on a file that very
// much has it. That is what mapGenericRights is for, and getting it wrong costs correctness twice
// over: EnsureGrant would rewrite an entire install tree's ACL on every plugin start, and any
// caller reading this as "no access" would be wrong.
func (c *Container) HasGrant(path string, mask uint32) (bool, error) {
	dacl, err := currentDACL(path)
	if err != nil {
		return false, err
	}
	if dacl == nil {
		// A nil DACL grants everyone everything. It is not a state this code creates, and reporting
		// "already granted" for it would silently accept a wide-open directory.
		return false, nil
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return false, fmt.Errorf("read ace %d of %s: %w", i, path, err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(c.sid) {
			continue
		}
		if granted := uint32(ace.Mask); granted&mask == mask || granted&mapGenericRights(mask) == mapGenericRights(mask) {
			return true, nil
		}
	}
	return false, nil
}

// mapGenericRights expands the generic bits of a file-system access mask into the specific rights
// Windows stores in an ACE. It is the GENERIC_MAPPING for files, applied by hand because x/sys does
// not wrap MapGenericMask.
func mapGenericRights(mask uint32) uint32 {
	mapped := mask &^ uint32(windows.GENERIC_READ|windows.GENERIC_WRITE|windows.GENERIC_EXECUTE)
	if mask&windows.GENERIC_READ != 0 {
		mapped |= windows.FILE_GENERIC_READ
	}
	if mask&windows.GENERIC_WRITE != 0 {
		mapped |= windows.FILE_GENERIC_WRITE
	}
	if mask&windows.GENERIC_EXECUTE != 0 {
		mapped |= windows.FILE_GENERIC_EXECUTE
	}
	return mapped
}

func currentDACL(path string) (*windows.ACL, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, fmt.Errorf("read security info of %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, fmt.Errorf("read dacl of %s: %w", path, err)
	}
	return dacl, nil
}
