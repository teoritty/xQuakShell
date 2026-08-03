package usecase

import "time"

// SetPreBindTimeoutForTest overrides the pre-bind eviction timeout (unit tests only).
func (s *TunnelDynamicService) SetPreBindTimeoutForTest(d time.Duration) {
	if s != nil {
		s.preBindTimeout = d
	}
}
