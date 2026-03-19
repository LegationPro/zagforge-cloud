package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LegationPro/zagforge/shared/go/router"
)

func ok(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func request(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestNew_returnsNonNilRouter(t *testing.T) {
	r := router.New()
	if r == nil {
		t.Fatal("expected non-nil router")
	}
}

func TestHandler_returnsNonNilHandler(t *testing.T) {
	r := router.New()
	if r.Handler() == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestRouter_registersAllMethods(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		reg    func(r *router.Router)
	}{
		{
			name:   "GET",
			method: http.MethodGet,
			path:   "/get",
			reg:    func(r *router.Router) { r.Get("/get", ok) },
		},
		{
			name:   "POST",
			method: http.MethodPost,
			path:   "/post",
			reg:    func(r *router.Router) { r.Post("/post", ok) },
		},
		{
			name:   "PUT",
			method: http.MethodPut,
			path:   "/put",
			reg:    func(r *router.Router) { r.Put("/put", ok) },
		},
		{
			name:   "DELETE",
			method: http.MethodDelete,
			path:   "/delete",
			reg:    func(r *router.Router) { r.Delete("/delete", ok) },
		},
		{
			name:   "PATCH",
			method: http.MethodPatch,
			path:   "/patch",
			reg:    func(r *router.Router) { r.Patch("/patch", ok) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := router.New()
			tt.reg(r)

			w := request(t, r.Handler(), tt.method, tt.path)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200 for %s %s, got %d", tt.method, tt.path, w.Code)
			}
		})
	}
}

func TestRouter_unmatchedRoute_returns405(t *testing.T) {
	r := router.New()
	r.Get("/only-get", ok)

	w := request(t, r.Handler(), http.MethodPost, "/only-get")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for wrong method, got %d", w.Code)
	}
}

func TestRouter_unknownPath_returns404(t *testing.T) {
	r := router.New()
	r.Get("/exists", ok)

	w := request(t, r.Handler(), http.MethodGet, "/does-not-exist")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown path, got %d", w.Code)
	}
}

func TestRouter_Use_appliesMiddleware(t *testing.T) {
	r := router.New()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-Test", "applied")
			next.ServeHTTP(w, req)
		})
	})
	r.Get("/mw", ok)

	w := request(t, r.Handler(), http.MethodGet, "/mw")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-Test"); got != "applied" {
		t.Errorf("expected middleware header %q, got %q", "applied", got)
	}
}

func TestRouter_Group_returnsNonNilGroup(t *testing.T) {
	r := router.New()
	g := r.Group()
	if g == nil {
		t.Fatal("expected non-nil group")
	}
}
