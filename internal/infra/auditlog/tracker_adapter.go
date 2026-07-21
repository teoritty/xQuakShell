package auditlog

import "xquakshell/internal/domain"

type trackerAdapter struct {
	inner *CommandLineTracker
}

func (t *trackerAdapter) Feed(data string) (string, bool) {
	return t.inner.Feed(data)
}

// NewCommandLineTrackerFactory returns a domain factory for command line trackers.
func NewCommandLineTrackerFactory() domain.CommandLineTrackerFactory {
	return func() domain.CommandLineTracker {
		return &trackerAdapter{inner: NewCommandLineTracker()}
	}
}
