package githubauth

import (
	"errors"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	dbpkg "github.com/LegationPro/zagforge/api/internal/db"
)

var ErrMissingInstallationId = errors.New("missing installation_id")

type Handler struct {
	db      *dbpkg.DB
	appSlug string
	log     *zap.Logger
}

func NewHandler(db *dbpkg.DB, appSlug string, log *zap.Logger) *Handler {
	return &Handler{db: db, appSlug: appSlug, log: log}
}

// Install redirects the user to the GitHub App installation page.
func (h *Handler) Install(w http.ResponseWriter, r *http.Request) {
	installURL := fmt.Sprintf("https://github.com/apps/%s/installations/new", h.appSlug)
	http.Redirect(w, r, installURL, http.StatusFound)
}

// Callback handles the redirect from GitHub after a user installs the app.
// GitHub sends ?installation_id=<id>&setup_action=install|update
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	installationID := r.URL.Query().Get("installation_id")
	setupAction := r.URL.Query().Get("setup_action")

	if installationID == "" {
		http.Error(w, ErrMissingInstallationId.Error(), http.StatusBadRequest)
		return
	}

	h.log.Info("github app callback",
		zap.String("installation_id", installationID),
		zap.String("setup_action", setupAction),
	)

	// TODO: store installation_id, sync repos, redirect to frontend dashboard
	fmt.Fprintf(w, "Installation %s successful (action: %s)", installationID, setupAction)
}
