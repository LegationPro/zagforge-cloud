package router_test

import (
	"net/http"
	"testing"

	"github.com/LegationPro/zagforge/shared/go/router"
)

func TestGroup_Create_registersAllMethods(t *testing.T) {
	tests := []struct {
		name   string
		method router.Method
		http   string
		path   string
	}{
		{"GET", router.GET, http.MethodGet, "/g-get"},
		{"POST", router.POST, http.MethodPost, "/g-post"},
		{"PUT", router.PUT, http.MethodPut, "/g-put"},
		{"DELETE", router.DELETE, http.MethodDelete, "/g-delete"},
		{"PATCH", router.PATCH, http.MethodPatch, "/g-patch"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := router.New()
			g := r.Group()

			err := g.Create([]router.Subroute{
				{Method: tt.method, Path: tt.path, Handler: ok},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			w := request(t, r.Handler(), tt.http, tt.path)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s %s, got %d", tt.http, tt.path, w.Code)
			}
		})
	}
}

func TestGroup_Create_multipleSubroutes(t *testing.T) {
	r := router.New()
	g := r.Group()

	err := g.Create([]router.Subroute{
		{Method: router.GET, Path: "/a", Handler: ok},
		{Method: router.POST, Path: "/b", Handler: ok},
		{Method: router.PUT, Path: "/c", Handler: ok},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/a"},
		{http.MethodPost, "/b"},
		{http.MethodPut, "/c"},
	} {
		w := request(t, r.Handler(), tc.method, tc.path)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 for %s %s, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestGroup_Create_invalidMethod_returnsError(t *testing.T) {
	r := router.New()
	g := r.Group()

	err := g.Create([]router.Subroute{
		{Method: "INVALID", Path: "/bad", Handler: ok},
	})
	if err == nil {
		t.Fatal("expected error for invalid method, got nil")
	}
}

func TestGroup_Create_mixedValidAndInvalid_registersValidAndReturnsError(t *testing.T) {
	r := router.New()
	g := r.Group()

	err := g.Create([]router.Subroute{
		{Method: router.GET, Path: "/valid", Handler: ok},
		{Method: "BOGUS", Path: "/invalid", Handler: ok},
	})
	if err == nil {
		t.Fatal("expected error for invalid method, got nil")
	}

	// The valid route should still be registered.
	w := request(t, r.Handler(), http.MethodGet, "/valid")
	if w.Code != http.StatusOK {
		t.Fatalf("expected valid route to be registered, got %d", w.Code)
	}
}

func TestGroup_Create_emptySubroutes_noError(t *testing.T) {
	r := router.New()
	g := r.Group()

	err := g.Create([]router.Subroute{})
	if err != nil {
		t.Fatalf("expected no error for empty subroutes, got %v", err)
	}
}

func TestGroup_Use_appliesMiddlewareToGroup(t *testing.T) {
	r := router.New()
	g := r.Group()
	g.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Group-MW", "yes")
			next.ServeHTTP(w, req)
		})
	})

	err := g.Create([]router.Subroute{
		{Method: router.GET, Path: "/with-mw", Handler: ok},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w := request(t, r.Handler(), http.MethodGet, "/with-mw")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-Group-MW"); got != "yes" {
		t.Errorf("expected group middleware header %q, got %q", "yes", got)
	}
}

func TestGroup_middlewareDoesNotAffectOtherGroups(t *testing.T) {
	r := router.New()

	// Group with middleware.
	g1 := r.Group()
	g1.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-G1", "yes")
			next.ServeHTTP(w, req)
		})
	})
	if err := g1.Create([]router.Subroute{
		{Method: router.GET, Path: "/g1", Handler: ok},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Group without middleware.
	g2 := r.Group()
	if err := g2.Create([]router.Subroute{
		{Method: router.GET, Path: "/g2", Handler: ok},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// g1 should have middleware header.
	w1 := request(t, r.Handler(), http.MethodGet, "/g1")
	if w1.Header().Get("X-G1") != "yes" {
		t.Error("expected g1 middleware to apply")
	}

	// g2 should NOT have g1's middleware header.
	w2 := request(t, r.Handler(), http.MethodGet, "/g2")
	if w2.Header().Get("X-G1") != "" {
		t.Error("g1 middleware leaked to g2")
	}
}

func TestGroup_middlewareBlocksRequest(t *testing.T) {
	r := router.New()
	g := r.Group()
	g.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		})
	})

	if err := g.Create([]router.Subroute{
		{Method: router.GET, Path: "/blocked", Handler: ok},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	w := request(t, r.Handler(), http.MethodGet, "/blocked")
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 from blocking middleware, got %d", w.Code)
	}
}

func TestGroup_handlerReceivesRequestData(t *testing.T) {
	r := router.New()
	g := r.Group()

	var gotPath string
	err := g.Create([]router.Subroute{
		{Method: router.GET, Path: "/echo", Handler: func(w http.ResponseWriter, req *http.Request) {
			gotPath = req.URL.Path
			w.WriteHeader(http.StatusOK)
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	request(t, r.Handler(), http.MethodGet, "/echo")
	if gotPath != "/echo" {
		t.Errorf("expected handler to receive path %q, got %q", "/echo", gotPath)
	}
}
