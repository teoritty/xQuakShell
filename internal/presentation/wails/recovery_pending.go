package wails

import "sync"

// pendingRecoveryKey holds a freshly issued recovery key for exactly as long as the one-time dialog
// showing it is open.
//
// The key is never written anywhere it could be read back - not to the vault, not to a settings
// file, not to the audit log. This is the only copy the application holds, and it exists so that
// the Save As handler can write the file without the key making a round trip through the WebView
// and back. A key that came back from JavaScript would be a key anything running in the WebView
// could ask to have written to disk.
//
// take() is deliberately destructive on acknowledgement: once the user presses Done, there is
// nothing left to copy, save, or show again, and no handler can resurrect it.
type pendingRecoveryKey struct {
	mu  sync.Mutex
	key string
}

// hold stores a newly issued key, replacing any earlier one.
func (p *pendingRecoveryKey) hold(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.key = key
}

// peek returns the held key without clearing it, for the Save As handler.
func (p *pendingRecoveryKey) peek() (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.key, p.key != ""
}

// clear forgets the key. It is what makes "this is shown once" true rather than a promise the UI
// makes on its own.
func (p *pendingRecoveryKey) clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.key = ""
}
