package usecase

import "sync"

const (
	freeFailures     = 3
	baseBackoff      = 1_000
	maxBackoff       = 5 * 60_000
	userHourlyBudget = 30
	userSlowSpacing  = 2 * 60_000
	hourMillis       = 60 * 60_000
	maxTrackedPairs  = 100_000
)

type pairState struct {
	failures int
	until    uint64
}

type userState struct {
	windowStart uint64
	failures    int
	last        uint64
}

// limiter throttles password guessing without ever locking an account. Failures from one address
// against one username back off exponentially; failures against one username from everywhere are
// slowed once they pass an hourly budget, but a correct attempt is always eventually possible - a
// lock-out would let anyone deny the owner their own account by guessing badly on purpose.
type limiter struct {
	mu    sync.Mutex
	pairs map[[2]string]*pairState
	users map[string]*userState
}

func newLimiter() *limiter {
	return &limiter{pairs: map[[2]string]*pairState{}, users: map[string]*userState{}}
}

// check returns how long the caller must still wait, zero when it may try now.
func (l *limiter) check(username, ip string, now uint64) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if p := l.pairs[[2]string{username, ip}]; p != nil && p.until > now {
		return p.until - now
	}
	u := l.users[username]
	if u == nil || now-u.windowStart >= hourMillis || u.failures < userHourlyBudget {
		return 0
	}
	if since := now - u.last; since < userSlowSpacing {
		return userSlowSpacing - since
	}
	return 0
}

// fail charges one attempt. LoginStart charges every attempt up front and succeed refunds the ones
// that verified, so what stays charged is exactly the attempts that did not prove the password.
func (l *limiter) fail(username, ip string, now uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.pairs) >= maxTrackedPairs || len(l.users) >= maxTrackedPairs {
		l.sweep(now)
	}
	key := [2]string{username, ip}
	p := l.pairs[key]
	if p == nil {
		p = &pairState{}
		l.pairs[key] = p
	}
	p.failures++
	if p.failures >= freeFailures {
		backoff := uint64(baseBackoff) << min(p.failures-freeFailures, 20)
		p.until = now + min(backoff, maxBackoff)
	}
	u := l.users[username]
	if u == nil || now-u.windowStart >= hourMillis {
		u = &userState{windowStart: now}
		l.users[username] = u
	}
	u.failures++
	u.last = now
}

// succeed refunds the charge of a login that verified: the address's backoff is cleared, and the
// username's hourly count gives back the one attempt that turned out to be the owner's.
func (l *limiter) succeed(username, ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.pairs, [2]string{username, ip})
	if u := l.users[username]; u != nil && u.failures > 0 {
		u.failures--
	}
}

// sweep drops entries that no longer slow anyone, so an attacker spraying addresses or usernames
// cannot grow the tables without bound. A username still over its hourly budget is kept: forgetting
// it is exactly what flooding the table with fresh names would be trying to achieve.
func (l *limiter) sweep(now uint64) {
	for k, p := range l.pairs {
		if p.until <= now {
			delete(l.pairs, k)
		}
	}
	for k, u := range l.users {
		if now-u.windowStart >= hourMillis || u.failures < userHourlyBudget {
			delete(l.users, k)
		}
	}
}
