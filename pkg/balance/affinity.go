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

type SessionAffinityBalancer struct {
	balancer      LoadBalancer
	sessions      map[string]*session
	mu            sync.RWMutex
	timeout       time.Duration
	cleanupTicker *time.Ticker
	stopCleanup   chan struct{}
}

func NewSessionAffinity(balancer LoadBalancer, timeout time.Duration) (*SessionAffinityBalancer, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("timeout must be greater than zero")
	}

	sa := &SessionAffinityBalancer{
		balancer:    balancer,
		sessions:    make(map[string]*session),
		timeout:     timeout,
		stopCleanup: make(chan struct{}),
	}

	sa.cleanupTicker = time.NewTicker(1 * time.Minute)
	go sa.cleanupLoop()
	return sa, nil
}

func (sa *SessionAffinityBalancer) SelectWithClientIP(clientIP string) (*node.Node, error) {
	sa.mu.Lock()
	sess, exists := sa.sessions[clientIP]

	// Check if session exists and is still valid
	if exists && time.Since(sess.lastAccess) < sa.timeout && sess.node.IsHealthy() {
		// Update timestamp and return existing node
		sess.lastAccess = time.Now()
		selectedNode := sess.node
		sa.mu.Unlock()
		return selectedNode, nil
	}
	sa.mu.Unlock()

	// No valid session - select new node using underlying balancer
	selectedNode, err := sa.balancer.Select()
	if err != nil {
		return nil, err
	}

	// Store new session
	sa.mu.Lock()
	sa.sessions[clientIP] = &session{
		node:       selectedNode,
		lastAccess: time.Now(),
	}
	sa.mu.Unlock()

	return selectedNode, nil
}

func (sa *SessionAffinityBalancer) Select() (*node.Node, error) {
	return sa.balancer.Select()
}

func (sa *SessionAffinityBalancer) Name() LoadBalancerType {
	return SessionAffinity
}

func (sa *SessionAffinityBalancer) cleanupLoop() {
	for {
		select {
		case <-sa.cleanupTicker.C:
			sa.cleanup()
		case <-sa.stopCleanup:
			sa.cleanupTicker.Stop()
			return
		}
	}
}

func (sa *SessionAffinityBalancer) cleanup() {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	for clientIP, sess := range sa.sessions {
		if time.Since(sess.lastAccess) > sa.timeout {
			delete(sa.sessions, clientIP)
		}
	}
}

// ClearSession removes a specific client's session
func (sa *SessionAffinityBalancer) ClearSession(clientIP string) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	delete(sa.sessions, clientIP)
}

// Stop stops the cleanup goroutine and clears all sessions
func (sa *SessionAffinityBalancer) Stop() {
	close(sa.stopCleanup)

	// Clear all sessions on shutdown
	sa.mu.Lock()
	sa.sessions = make(map[string]*session)
	sa.mu.Unlock()
}
