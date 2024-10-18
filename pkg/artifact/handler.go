//go:build !solution

package artifact

import (
	"errors"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/tarstream"
	"net/http"

	"go.uber.org/zap"
)

type Handler struct {
	logger *zap.Logger
	cache  *Cache
}

func NewHandler(l *zap.Logger, c *Cache) *Handler {
	return &Handler{
		logger: l,
		cache:  c,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/artifact", h.getArtifact)
}

func (h *Handler) getArtifact(w http.ResponseWriter, r *http.Request) {
	artifactID := r.URL.Query().Get("id")
	if artifactID == "" {
		h.handleError(w, http.StatusBadRequest, errors.New("missing artifact id"))
		return
	}

	var id build.ID
	err := id.UnmarshalText([]byte(artifactID))
	if err != nil {
		h.logger.Error("failed to unmarshal artifact id", zap.String("artifact_id", artifactID))
		h.handleError(w, http.StatusBadRequest, err)
		return
	}

	path, unlock, err := h.cache.Get(id)
	if err != nil {
		h.logger.Error("failed to get artifact from cache", zap.String("artifact_id", artifactID), zap.Error(err))
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}
	defer unlock()

	err = tarstream.Send(path, w)
	if err != nil {
		h.logger.Error("failed to receive artifact from cache", zap.String("artifact_id", artifactID), zap.Error(err))
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	if err != nil {
		w.Write([]byte(err.Error()))
		h.logger.Error("failed to handle request", zap.Error(err))
	}
}
