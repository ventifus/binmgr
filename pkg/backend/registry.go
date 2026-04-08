package backend

import (
	"fmt"
	"net/url"
)

type Registry struct {
	backends []Backend
}

func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a backend to the registry in registration order.
func (r *Registry) Register(b Backend) {
	r.backends = append(r.backends, b)
}

// Dispatch returns the first backend whose CanHandle returns true for u.
// Returns an error if no backend matches.
func (r *Registry) Dispatch(u *url.URL) (Backend, error) {
	for _, b := range r.backends {
		if b.CanHandle(u) {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no backend found for URL %q", u.String())
}

// DispatchByType returns the backend whose Type() matches t.
// Returns an error if no backend matches.
func (r *Registry) DispatchByType(t string) (Backend, error) {
	for _, b := range r.backends {
		if b.Type() == t {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no backend registered with type %q", t)
}
