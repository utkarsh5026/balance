package security

import "fmt"

type SecurityManager struct {
	rateLimiter RateLimiter
	ip          *IPBlocklist
}

func NewSecurityManager(rl RateLimiter, ip *IPBlocklist) *SecurityManager {
	return &SecurityManager{
		rateLimiter: rl,
		ip:          ip,
	}
}

func (sm *SecurityManager) AllowConn(ip string) (bool, error) {
	if sm.ip.IsBlocked(ip) {
		return false, fmt.Errorf("ip address blocked")
	}

	if sm.rateLimiter != nil && !sm.rateLimiter.Allow(ip) {
		return false, fmt.Errorf("rate limit exceeded")
	}
	return true, nil
}
