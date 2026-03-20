package router

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Compile-time check
var _ Grouper = (*Group)(nil)

type Method string

const (
	GET    Method = "GET"
	POST   Method = "POST"
	PUT    Method = "PUT"
	DELETE Method = "DELETE"
	PATCH  Method = "PATCH"
)

type Subroute struct {
	Method  Method
	Path    string
	Handler http.HandlerFunc
}

type Group struct {
	mux *chi.Mux
	mws []func(http.Handler) http.Handler
}

func NewGroup(mux *chi.Mux) *Group {
	return &Group{mux: mux}
}

// Use appends middleware to the group's middleware stack.
func (g *Group) Use(mw func(http.Handler) http.Handler) {
	g.mws = append(g.mws, mw)
}

// Create registers all subroutes on the group's mux.
// If middleware is set via Use, routes are wrapped in a chi group with that middleware.
func (g *Group) Create(subroutes []Subroute) error {
	var errs []error

	methods := map[Method]func(chi.Router, string, http.HandlerFunc){
		GET:    func(r chi.Router, p string, h http.HandlerFunc) { r.Get(p, h) },
		POST:   func(r chi.Router, p string, h http.HandlerFunc) { r.Post(p, h) },
		PUT:    func(r chi.Router, p string, h http.HandlerFunc) { r.Put(p, h) },
		DELETE: func(r chi.Router, p string, h http.HandlerFunc) { r.Delete(p, h) },
		PATCH:  func(r chi.Router, p string, h http.HandlerFunc) { r.Patch(p, h) },
	}

	register := func(r chi.Router) {
		for _, sub := range subroutes {
			if fn, ok := methods[sub.Method]; ok {
				fn(r, sub.Path, sub.Handler)
			} else {
				errs = append(errs,
					fmt.Errorf("failed to register path: %q with method: %q", sub.Path, sub.Method),
				)
			}
		}
	}

	if len(g.mws) > 0 {
		g.mux.Group(func(r chi.Router) {
			for _, mw := range g.mws {
				r.Use(mw)
			}
			register(r)
		})
	} else {
		register(g.mux)
	}

	return errors.Join(errs...)
}

// Grouper is the interface satisfied by Group.
type Grouper interface {
	Create(subroutes []Subroute) error
}
