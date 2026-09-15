package route

import (
	"erent/internal/modules/translator"
	"errors"
	"maps"
	"sync"
)

type AuthMode string

const (
	AuthModeAccessToken AuthMode = "access_token"
	AuthModeAPIKey      AuthMode = "api_key"
)

type Route struct {
	// Provider      string
	// UpstreamModel string
	UpstreamFormat translator.Format
	Endpoint       string
	ForceStream    bool
	AuthHeader     string            // 凭证请求头名称，例如 Authorization。
	AuthPrefix     string            // 凭证前缀，例如 "Bearer "。
	Headers        map[string]string // 固定请求头，不保存本次请求的凭证。
}

type Registry struct {
	mu     sync.RWMutex
	routes map[string]map[AuthMode]map[translator.Format]Route
}

func NewRegistry() *Registry {
	return &Registry{routes: make(map[string]map[AuthMode]map[translator.Format]Route)}
}

func (r *Registry) register(model string, mode AuthMode, clientFormat translator.Format, route Route) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.routes[model] == nil {
		r.routes[model] = make(map[AuthMode]map[translator.Format]Route)
	}
	if r.routes[model][mode] == nil {
		r.routes[model][mode] = make(map[translator.Format]Route)
	}
	route.Headers = maps.Clone(route.Headers)
	r.routes[model][mode][clientFormat] = route
}

var defaultRegistry = NewRegistry()

func Register(model string, mode AuthMode, clientFormat translator.Format, route Route) {
	defaultRegistry.register(model, mode, clientFormat, route)
}

func (r *Registry) getRoute(model string, mode AuthMode, clientFormat translator.Format) (Route, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	route, ok := r.routes[model][mode][clientFormat]
	if !ok {
		return Route{}, errors.ErrUnsupported
	}
	route.Headers = maps.Clone(route.Headers)
	return route, nil
}

func GetRoute(model string, mode AuthMode, clientFormat translator.Format) (Route, error) {
	return defaultRegistry.getRoute(model, mode, clientFormat)
}
