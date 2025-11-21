package router

import (
	"net/http"
	"sort"
	"strings"

	"github.com/utkarsh5026/balance/pkg/conf"
	"github.com/utkarsh5026/balance/pkg/node"
)

// RouteEntry represents a compiled route with its backend pool
type RouteEntry struct {
	config conf.Route
	pool   *node.Pool
}

type Router struct {
	routes      []RouteEntry
	defaultPool *node.Pool
}

func NewRouter(routes []conf.Route, defaultPool *node.Pool) *Router {
	r := &Router{
		defaultPool: defaultPool,
		routes:      make([]RouteEntry, 0, len(routes)),
	}

	for _, rt := range routes {
		pool := node.NewPool()

		for _, bName := range rt.Backends {
			if b := defaultPool.GetByName(bName); b != nil {
				pool.Add(b)
			}
		}

		r.routes = append(r.routes, RouteEntry{
			config: rt,
			pool:   pool,
		})
	}

	sort.Slice(r.routes, func(i, j int) bool {
		return r.routes[i].config.Priority < r.routes[j].config.Priority
	})

	return r
}

func (r *Router) Match(req *http.Request) *node.Pool {
	for _, route := range r.routes {
		if r.matchRoute(req, &route.config) {
			return route.pool
		}
	}

	// No route matched, use default pool
	return r.defaultPool
}
func (r *Router) matchRoute(req *http.Request, route *conf.Route) bool {
	// Check host matching
	if route.Host != "" {
		if !matchHost(req.Host, route.Host) {
			return false
		}
	}

	// Check path prefix matching
	if route.PathPrefix != "" {
		if !strings.HasPrefix(req.URL.Path, route.PathPrefix) {
			return false
		}
	}

	// Check header matching
	if len(route.Headers) > 0 {
		for key, value := range route.Headers {
			if req.Header.Get(key) != value {
				return false
			}
		}
	}

	return true
}

func matchHost(reqHost, routeHost string) bool {
	if idx := strings.Index(reqHost, ":"); idx != -1 {
		reqHost = reqHost[:idx]
	}

	if reqHost == routeHost {
		return true
	}

	if strings.HasPrefix(routeHost, "*.") {
		suffix := routeHost[1:] // Remove the *
		return strings.HasSuffix(reqHost, suffix)
	}

	return false
}
