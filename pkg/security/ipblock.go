package security

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type IPBlocklist struct {
	mu sync.RWMutex

	// blocked maps IP addresses to block expiry time
	blocked map[string]time.Time

	// Permanent blocks (never expire)
	permanent map[string]bool
}

func NewIPBlocklist(ctx context.Context) *IPBlocklist {
	bl := &IPBlocklist{
		blocked:   make(map[string]time.Time),
		permanent: make(map[string]bool),
	}

	go bl.removeBlockedPeriodically(ctx)
	return bl
}

func (i *IPBlocklist) removeBlockedPeriodically(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			i.mu.Lock()
			now := time.Now()

			toDelete := make([]string, 0, len(i.blocked))
			for ip, expiry := range i.blocked {
				if now.After(expiry) {
					toDelete = append(toDelete, ip)
				}
			}

			for _, ip := range toDelete {
				delete(i.blocked, ip)
			}

			i.mu.Unlock()
		}
	}
}

// IsBlocked checks if an IP address is blocked
func (bl *IPBlocklist) IsBlocked(ip string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()

	if bl.permanent[ip] {
		return true
	}

	if expiry, exists := bl.blocked[ip]; exists {
		if time.Now().Before(expiry) {
			return true
		}
	}

	return false
}

func (bl *IPBlocklist) Unblock(ip string) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	delete(bl.permanent, ip)
	delete(bl.blocked, ip)
	slog.Info("Unblocked ", "IP", ip)
}

// BlockPermanent permanently blocks an IP address
func (bl *IPBlocklist) BlockPermanent(ip string) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	bl.permanent[ip] = true

	slog.Info("Permanently blocked", "IP", ip)
}
