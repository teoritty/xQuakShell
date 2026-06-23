package domain

// PuTTYSession is a parsed PuTTY session entry from a registry export.
type PuTTYSession struct {
	Name     string
	HostName string
	Port     int
	UserName string
	Protocol string
}

// PuTTYImporter parses PuTTY registry exports and converts PPK key files.
type PuTTYImporter interface {
	ParseReg(content string) ([]PuTTYSession, error)
	PPKToPEM(ppkContent []byte, passphrase string) ([]byte, string, error)
}
