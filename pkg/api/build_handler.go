//go:build !solution

package api

import (
	"encoding/json"
	"errors"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"io"
	"net/http"

	"go.uber.org/zap"
)

func NewBuildService(l *zap.Logger, s Service) *BuildHandler {
	return &BuildHandler{
		logger:  l,
		service: s,
	}
}

type BuildHandler struct {
	logger  *zap.Logger
	service Service
}

func (h *BuildHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/build", h.build)
	mux.HandleFunc("/signal", h.signal)
}

func (h *BuildHandler) build(w http.ResponseWriter, r *http.Request) {
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		h.handleError(w, http.StatusBadRequest, err)
		return
	}
	defer r.Body.Close()

	var buildRequest BuildRequest
	err = json.Unmarshal(requestData, &buildRequest)
	if err != nil {
		h.handleError(w, http.StatusBadRequest, err)
	}

	statusWriter := newStatusWriter(h.logger, w)
	err = h.service.StartBuild(r.Context(), &buildRequest, statusWriter)
	if err != nil {
		if w.Header().Get("Content-Type") == "application/json" {
			statusWriter.Updated(&StatusUpdate{
				BuildFailed: &BuildFailed{
					Error: err.Error(),
				},
			})
		} else {
			h.handleError(w, http.StatusInternalServerError, err)
		}
	}
}

func (h *BuildHandler) signal(w http.ResponseWriter, r *http.Request) {
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		h.handleError(w, http.StatusBadRequest, err)
		return
	}
	defer r.Body.Close()

	var signalRequest SignalRequest
	err = json.Unmarshal(requestData, &signalRequest)
	if err != nil {
		h.handleError(w, http.StatusBadRequest, err)
		return
	}

	buildID := r.URL.Query().Get("build_id")
	if buildID == "" {
		h.handleError(w, http.StatusBadRequest, errors.New("build_id is required in url"))
		return
	}

	var id build.ID
	err = id.UnmarshalText([]byte(buildID))
	if err != nil {
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}

	signalResponse, err := h.service.SignalBuild(r.Context(), id, &signalRequest)
	if err != nil {
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}

	responseData, err := json.Marshal(&signalResponse)
	if err != nil {
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	w.Write(responseData)
}

func (h *BuildHandler) handleError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	if err != nil {
		w.Write([]byte(err.Error()))
		h.logger.Error(err.Error())
	}
}
