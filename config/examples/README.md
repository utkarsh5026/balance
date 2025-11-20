# Load Balancer Configuration Examples

This directory contains example configurations for all available load balancing algorithms.

## 📊 Performance Comparison

Based on benchmark results (operations per second, higher is better):

| Algorithm | Performance | Ops/sec | Best For |
|-----------|------------|---------|----------|
| **Session Affinity** (cached) | ⚡⚡⚡ | ~14.7M | Existing sticky sessions |
| **Least Connections Weighted** | ⚡⚡⚡ | ~10.3M | Production with varying capacities |
| **Round Robin** | ⚡⚡⚡ | ~10.9M | Simple equal distribution |
| **Least Connections** | ⚡⚡⚡ | ~7.4M | Dynamic workloads |
| **Consistent Hash** | ⚡⚡ | ~2.9M | Session persistence, caching |
| **Bounded Consistent Hash** | ⚡⚡ | ~2.5M | Hash-based with load balancing |
| **Session Affinity** (new) | ⚡⚡ | ~2.4M | New session creation |
| **Weighted Round Robin** | ⚡ | ~1.0M | Smooth weighted distribution |

## 🎯 Algorithm Selection Guide

### Use **Round Robin** when:
- ✅ All backends have similar capacity
- ✅ Requests have similar processing time
- ✅ You need the simplest, fastest solution
- ✅ No session persistence required
- 📊 Performance: ~130 ns/op, 10.9M ops/sec

### Use **Weighted Round Robin** when:
- ✅ Backends have different capacities
- ✅ You want smooth distribution (NGINX-style)
- ✅ You need predictable traffic patterns
- ⚠️ Warning: Slower with high concurrency (mutex contention)
- 📊 Performance: ~1,325 ns/op, 1.0M ops/sec

### Use **Least Connections** when:
- ✅ Requests have varying processing times
- ✅ You want dynamic load balancing
- ✅ All backends have similar capacity
- ✅ Need to avoid overloading any single backend
- 📊 Performance: ~157 ns/op, 7.4M ops/sec

### Use **Least Connections Weighted** when:
- ✅ **Best overall choice for production!**
- ✅ Backends have different capacities
- ✅ Requests have varying processing times
- ✅ You want intelligent load distribution
- 📊 Performance: ~125 ns/op, 10.3M ops/sec (FASTEST WEIGHTED!)

### Use **Consistent Hash** when:
- ✅ Need session persistence/affinity
- ✅ Running stateful applications
- ✅ Implementing cache layers
- ✅ Same client should always hit same backend
- 📊 Performance: ~407 ns/op, 2.9M ops/sec
- ⚙️ Uses 200 virtual nodes per backend

### Use **Bounded Consistent Hash** when:
- ✅ Need consistent hashing with load balancing
- ✅ Want to prevent hotspots
- ✅ Maintain session affinity when possible
- ✅ Automatically fallback when backend overloaded
- 📊 Performance: ~469 ns/op, 2.5M ops/sec
- ⚙️ Load factor: 1.25 (max 25% above average)

### Use **Session Affinity** when:
- ✅ **Fastest for sticky sessions!**
- ✅ Need IP-based session persistence
- ✅ Stateful applications
- ✅ Want automatic session cleanup
- 📊 Performance: ~78 ns/op cached, ~520 ns/op new
- ⚙️ Session timeout: 5 minutes

## 📁 Example Files

- [round-robin.yaml](round-robin.yaml) - Simple round-robin
- [weighted-round-robin.yaml](weighted-round-robin.yaml) - Weighted distribution
- [least-connections.yaml](least-connections.yaml) - Least active connections
- [least-connections-weighted.yaml](least-connections-weighted.yaml) - **Recommended!**
- [consistent-hash.yaml](consistent-hash.yaml) - Hash-based routing
- [bounded-consistent-hash.yaml](bounded-consistent-hash.yaml) - Hash + load balancing
- [session-affinity.yaml](session-affinity.yaml) - IP-based sticky sessions

## 🔧 Configuration Parameters

### Common Parameters

```yaml
load_balancer:
  algorithm: <algorithm-name>  # Required
  hash_key: source-ip          # Optional, for hash-based algorithms
```

### Algorithm Names

- `round-robin`
- `weighted-round-robin`
- `least-connections`
- `least-connections-weighted`
- `consistent-hash`
- `bounded-consistent-hash`
- `session-affinity`

### Hash Key Options (for consistent-hash algorithms)

- `source-ip` - Route based on client IP (default)
- Custom keys can be implemented for HTTP mode

## 📈 Scaling Behavior

### Round Robin
- **5 nodes**: 130 ns/op
- **10 nodes**: 159 ns/op (+22%)
- **50 nodes**: 317 ns/op (+143%)
- **100 nodes**: 581 ns/op (+345%)
- **Verdict**: ✅ Linear scaling

### Weighted Round Robin
- **5 nodes**: 429 ns/op
- **10 nodes**: 892 ns/op (+108%)
- **50 nodes**: 4,197 ns/op (+878%)
- **100 nodes**: 9,707 ns/op (+2,162%)
- **Verdict**: ⚠️ Poor scaling (mutex contention)

### Least Connections
- **5 nodes**: 133 ns/op
- **10 nodes**: 178 ns/op (+34%)
- **50 nodes**: 357 ns/op (+169%)
- **100 nodes**: 744 ns/op (+460%)
- **Verdict**: ✅ Good scaling

### Consistent Hash
- **5 nodes**: 339 ns/op
- **10 nodes**: 517 ns/op (+53%)
- **50 nodes**: 783 ns/op (+131%)
- **100 nodes**: 1,211 ns/op (+258%)
- **Verdict**: ✅ Reasonable scaling

## 💾 Memory Usage

All algorithms have **constant memory usage** regardless of pool size:

- Round Robin / Least Connections: 80 B/op, 1 alloc/op
- Consistent Hash: 96 B/op, 3 allocs/op
- Session Affinity (cached): 0 B/op, 0 allocs/op

## 🚀 Quick Start

1. Choose a configuration file from this directory
2. Copy it to your deployment location
3. Modify the backend addresses
4. Run the proxy:

```bash
balance -config /path/to/config.yaml
```

## 📝 Notes

- All benchmarks run on: 11th Gen Intel® Core™ i7-11800H @ 2.30GHz
- Performance may vary based on hardware and workload
- For production, consider **least-connections-weighted** for best balance of performance and intelligence
- **Session affinity** is fastest for cached sessions but requires session management
