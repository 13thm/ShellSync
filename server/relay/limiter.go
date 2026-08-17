package relay

import "time"

// ipLimiter is a sliding-window rate limiter with blacklist, used to brake
// pairing-code brute force on the public claim path.
//
// Policy (doc §6.2): 5 claims per minute per IP; exceeding the budget
// blacklists the IP for 10 minutes.
type ipLimiter struct {
	window    time.Duration
	max       int
	blackFor  time.Duration
	events    []time.Time
	blacklist time.Time // until
}

func newIPLimiter() *ipLimiter {
	return &ipLimiter{window: time.Minute, max: 5, blackFor: 10 * time.Minute}
}

// allow records one attempt and reports whether it is permitted. now is
// injected for testability.
func (l *ipLimiter) allow(now time.Time) bool {
	if now.Before(l.blacklist) {
		return false
	}
	// drop events older than the window
	keep := 0
	for _, e := range l.events {
		if now.Sub(e) < l.window {
			l.events[keep] = e
			keep++
		}
	}
	l.events = l.events[:keep]
	if len(l.events) >= l.max {
		l.blacklist = now.Add(l.blackFor)
		l.events = nil
		return false
	}
	l.events = append(l.events, now)
	return true
}

// blacklisted reports whether the limiter is currently blacklisted.
func (l *ipLimiter) blacklisted(now time.Time) bool {
	return now.Before(l.blacklist)
}
