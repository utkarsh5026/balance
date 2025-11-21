package security

import "fmt"

type SecurityManager struct {
	rateLimiter RateLimiter
}

func NewSecurityManager(rl RateLimiter) *SecurityManager {
	return &SecurityManager{
		rateLimiter: rl,
	}
}

func (sm *SecurityManager) AllowConn(ip string) (bool, error) {
	// Check rate limit
	if sm.rateLimiter != nil && !sm.rateLimiter.Allow(ip) {
		return false, fmt.Errorf("rate limit exceeded")
	}
	return true, nil
}
