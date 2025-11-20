package balance

import (
	"hash/fnv"
	"sort"
	"sync"

	"github.com/utkarsh5026/balance/pkg/node"
)

const (
	defaultVirtualNodes = 200
	defaultHashKey      = "default-hash-key"
)

type ConsistentHashBalancer struct {
	pool          *node.Pool
	virtualNodes  int
	ring          []uint32
	ringMap       map[uint32]*node.Node
	mu            sync.RWMutex
	lastKnownSize int
	hashKey       string
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
		hashKey:      hashKey,
	}

	c.syncRing()
	return c
}

func (c *ConsistentHashBalancer) Name() LoadBalancerType {
	return ConsistentHash
}

func (c *ConsistentHashBalancer) Select() (*node.Node, error) {
	return c.SelectWithKey("")
}

func (c *ConsistentHashBalancer) SelectWithKey(key string) (*node.Node, error) {
	c.syncRing()
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

func (c *ConsistentHashBalancer) syncRing() {
	c.mu.Lock()
	defer c.mu.Unlock()

	healthy := c.pool.Healthy()
	if len(healthy) == c.lastKnownSize && len(c.ring) > 0 {
		return
	}

	c.ring = make([]uint32, 0, len(healthy)*c.virtualNodes)
	c.ringMap = make(map[uint32]*node.Node)

	for _, n := range healthy {
		weight := properWeight(n)
		numVirtualNodes := c.virtualNodes * weight

		for i := 0; i < numVirtualNodes; i++ {
			virtualNodeKey := c.hashKey + n.Address() + string(i)
			hash := c.hash(virtualNodeKey)
			c.ring = append(c.ring, hash)
			c.ringMap[hash] = n
		}
	}

	c.lastKnownSize = len(healthy)
	sort.Slice(c.ring, func(i, j int) bool {
		return c.ring[i] < c.ring[j]
	})
}

func (c *ConsistentHashBalancer) hash(key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return h.Sum32()
}
