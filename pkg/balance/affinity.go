package balance

import (
	"fmt"
	"sync"
	"time"

	"github.com/utkarsh5026/balance/pkg/node"
)

type session struct {
	node       *node.Node
	lastAccess time.Time
}

type SessionAfinityBalancer struct {
	balancer      LoadBalancer
	sessions      map[string]*session
	mu            sync.RWMutex
	timeout       time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

func NewSessionAffinity(balancer LoadBalancer, timeout time.Duration) (*SessionAfinityBalancer, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be greater than zero")
	}

	sa := &SessionAfinityBalancer{
		balancer:    balancer,
		sessions:    make(map[string]*session),
		timeout:     timeout,
		stopCleanup: make(chan struct{}),
	}

	sa.cleanupTicker = time.NewTicker(1 * time.Minute)
	go sa.cleanupLoop()
	return sa, nil
}

func (sa *SessionAfinityBalancer) SelectWithClientIP(clientIP string) (*node.Node, error) {
	sa.mu.RLock()
	if sess, exists := sa.sessions[clientIP]; exists {
		if time.Since(sess.lastAccess) < sa.timeout && sess.node.IsHealthy() {
			sa.mu.RUnlock()

			sa.mu.Lock()
			sess.lastAccess = time.Now()
			sa.mu.Unlock()

			return sess.node, nil
		}
	}

	sa.mu.RUnlock()
	node, err := sa.balancer.Select()
	if err != nil {
		return nil, err
	}

	sa.mu.Lock()
	defer sa.mu.Unlock()

	sa.sessions[clientIP] = &session{
		node:       node,
		lastAccess: time.Now(),
	}

	return node, nil
}

func (sa *SessionAfinityBalancer) Select() (*node.Node, error) {
	return sa.balancer.Select()
}

func (sa *SessionAfinityBalancer) Name() LoadBalancerType {
	return SessionAffinity
}

func (sa *SessionAfinityBalancer) cleanupLoop() {
	for {
		select {
		case <-sa.cleanupTicker.C:
			sa.cleaup()
		case <-sa.stopCleanup:
			sa.cleanupTicker.Stop()
			return
		}
	}
}

func (sa *SessionAfinityBalancer) cleaup() {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	now := time.Now()
	for clientIP, sess := range sa.sessions {
		if now.Sub(sess.lastAccess) > sa.timeout {
			delete(sa.sessions, clientIP)
		}
	}
}

// ClearSession removes a specific client's session
func (sa *SessionAfinityBalancer) ClearSession(clientIP string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	delete(sa.sessions, clientIP)
}

func (sa *SessionAfinityBalancer) Stop() {
	close(sa.stopCleanup)
}
