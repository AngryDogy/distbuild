package api

import (
	"encoding/json"
	"go.uber.org/zap"
	"net/http"
)

type StatusWriter interface {
	Started(rsp *BuildStarted) error
	Updated(update *StatusUpdate) error
}

type statusWriter struct {
	logger     *zap.Logger
	w          http.ResponseWriter
	controller *http.ResponseController
}

func newStatusWriter(l *zap.Logger, w http.ResponseWriter) StatusWriter {
	return &statusWriter{
		logger:     l,
		w:          w,
		controller: http.NewResponseController(w),
	}
}

func (sw *statusWriter) Started(rsp *BuildStarted) error {
	data, err := json.Marshal(rsp)
	if err != nil {
		return err
	}

	sw.w.Header().Set("Content-Type", "application/json")
	_, err = sw.w.Write(data)
	if err != nil {
		return err
	}

	sw.logger.Info("wrote started status successfully")
	return sw.controller.Flush()
}

func (sw *statusWriter) Updated(status *StatusUpdate) error {
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}

	_, err = sw.w.Write(data)
	if err != nil {
		return err
	}

	sw.logger.Info("wrote updated status successfully")
	return sw.controller.Flush()
}
