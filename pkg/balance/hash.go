package balance

import (
	"fmt"
	"hash/fnv"
	"slices"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/utkarsh5026/balance/pkg/node"
)

const (
	defaultVirtualNodes = 200
	defaultHashKey      = "default-hash-key"
)

type ConsistentHashBalancer struct {
	pool         *node.Pool
	virtualNodes int
	ring         []uint32
	ringMap      map[uint32]*node.Node
	mu           sync.RWMutex
	nodeHashes   map[string]struct{}
	hashKey      string
	counter      uint64
}

func NewConsistentHashBalancer(pool *node.Pool, virtualNodes int, hashKey string) *ConsistentHashBalancer {
	if virtualNodes <= 0 {
		virtualNodes = defaultVirtualNodes
	}

	if hashKey == "" {
		hashKey = defaultHashKey
	}

	c := &ConsistentHashBalancer{
		pool:         pool,
		virtualNodes: virtualNodes,
		ringMap:      make(map[uint32]*node.Node),
		nodeHashes:   make(map[string]struct{}),
		hashKey:      hashKey,
	}

	c.syncRing()
	return c
}

func (c *ConsistentHashBalancer) Name() LoadBalancerType {
	return ConsistentHash
}

func (c *ConsistentHashBalancer) Select() (*node.Node, error) {
	count := atomic.AddUint64(&c.counter, 1)
	key := fmt.Sprintf("%d", count)
	return c.SelectWithKey(key)
}

func (c *ConsistentHashBalancer) SelectWithKey(key string) (*node.Node, error) {
	c.syncRingIfNeeded()

	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.ring) == 0 {
		return nil, ErrNoNodeHealthy
	}

	hash := c.hash(key)
	idx := sort.Search(len(c.ring), func(i int) bool {
		return c.ring[i] >= hash
	})

	if idx == len(c.ring) {
		idx = 0
	}

	return c.ringMap[c.ring[idx]], nil
}

func (c *ConsistentHashBalancer) syncRingIfNeeded() {
	c.mu.RLock()
	needsSync := c.hasPoolChanged()
	c.mu.RUnlock()

	if needsSync {
		c.syncRing()
	}
}

func (c *ConsistentHashBalancer) hasPoolChanged() bool {
	healthy := c.pool.Healthy()
	if len(healthy) != len(c.nodeHashes) {
		return true
	}

	for _, n := range healthy {
		if _, exists := c.nodeHashes[n.Address()]; !exists {
			return true
		}
	}

	return false
}

func (c *ConsistentHashBalancer) syncRing() {
	c.mu.Lock()
	defer c.mu.Unlock()

	healthy := c.pool.Healthy()

	c.ring = make([]uint32, 0, len(healthy)*c.virtualNodes)
	c.ringMap = make(map[uint32]*node.Node)
	c.nodeHashes = make(map[string]struct{})

	for _, n := range healthy {
		c.nodeHashes[n.Address()] = struct{}{}
		weight := properWeight(n)
		numVirtualNodes := c.virtualNodes * weight

		for i := range numVirtualNodes {
			virtualNodeKey := fmt.Sprintf("%s:%s:%d", c.hashKey, n.Address(), i)
			hash := c.hash(virtualNodeKey)
			c.ring = append(c.ring, hash)
			c.ringMap[hash] = n
		}
	}

	slices.Sort(c.ring)
}

func (c *ConsistentHashBalancer) hash(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}
