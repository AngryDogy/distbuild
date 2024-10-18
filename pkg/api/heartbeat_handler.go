//go:build !solution

package api

import (
	"encoding/json"
	"io"
	"net/http"

	"go.uber.org/zap"
)

type HeartbeatHandler struct {
	logger  *zap.Logger
	service HeartbeatService
}

func NewHeartbeatHandler(l *zap.Logger, s HeartbeatService) *HeartbeatHandler {
	return &HeartbeatHandler{
		logger:  l,
		service: s,
	}
}

func (h *HeartbeatHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		var heartbeatRequest HeartbeatRequest

		requestData, err := io.ReadAll(r.Body)
		if err != nil {
			h.handleError(w, http.StatusBadRequest, err)
			return
		}
		defer r.Body.Close()

		err = json.Unmarshal(requestData, &heartbeatRequest)
		if err != nil {
			h.handleError(w, http.StatusBadRequest, err)
			return
		}

		heartbeatResponse, err := h.service.Heartbeat(r.Context(), &heartbeatRequest)
		if err != nil {
			h.handleError(w, http.StatusInternalServerError, err)
			return
		}

		responseData, err := json.Marshal(heartbeatResponse)
		if err != nil {
			h.handleError(w, http.StatusInternalServerError, err)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set("Content-Type", "application/json")
		w.Write(responseData)

	})
}

func (h *HeartbeatHandler) handleError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	if err != nil {
		w.Write([]byte(err.Error()))
		h.logger.Error(err.Error())
	}
}
