//go:build !solution

package filecache

import (
	"errors"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"golang.org/x/sync/singleflight"
	"io"
	"net/http"
	"os"

	"go.uber.org/zap"
)

type Handler struct {
	logger       *zap.Logger
	cache        *Cache
	requestGroup singleflight.Group
}

func NewHandler(l *zap.Logger, cache *Cache) *Handler {
	return &Handler{
		logger:       l,
		cache:        cache,
		requestGroup: singleflight.Group{},
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/file", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			h.getFile(w, r)
		case "POST":
			h.requestGroup.Do(r.URL.Path, func() (interface{}, error) {
				h.uploadFile(w, r)
				return nil, nil
			})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte("Only GET and POST methods are supported"))
		}
	})
}

func (h *Handler) getFile(w http.ResponseWriter, r *http.Request) {
	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		h.handleError(w, http.StatusBadRequest, errors.New("no file id provided"))
		return
	}

	var id build.ID
	err := id.UnmarshalText([]byte(fileID))
	if err != nil {
		h.logger.Error("failed to unmarshal file id", zap.String("id", fileID))
		h.handleError(w, http.StatusBadRequest, err)
		return
	}

	path, unlock, err := h.cache.Get(id)
	if err != nil {
		h.logger.Error("failed to get file from cache", zap.String("id", fileID), zap.Error(err))
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}
	defer unlock()

	file, err := os.Open(path)
	if err != nil {
		h.logger.Error("failed to open file", zap.String("id", fileID), zap.Error(err))
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		h.logger.Error("failed to read file", zap.String("id", fileID), zap.Error(err))
		h.handleError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		h.logger.Error("failed to read request body", zap.Error(err))
		h.handleError(w, http.StatusBadRequest, err)
		return
	}
	defer r.Body.Close()

	fileID := r.URL.Query().Get("id")
	if fileID == "" {
		h.handleError(w, http.StatusBadRequest, errors.New("no file id provided"))
		return
	}

	var id build.ID
	err = id.UnmarshalText([]byte(fileID))
	if err != nil {
		h.logger.Error("failed to unmarshal file id", zap.String("id", fileID))
		h.handleError(w, http.StatusBadRequest, err)
		return
	}

	var writer io.WriteCloser
	var abort func() error
	for {
		writer, abort, err = h.cache.Write(id)

		if errors.Is(err, ErrExists) {
			err := h.cache.Remove(id)
			if err != nil {
				h.logger.Error("failed to remove file", zap.String("id", fileID), zap.Error(err))
				h.handleError(w, http.StatusInternalServerError, err)
				return
			}
			continue
		}

		if err != nil {
			h.logger.Error("failed to create writer", zap.String("id", fileID), zap.Error(err))
			h.handleError(w, http.StatusInternalServerError, err)
			return
		}
		break
	}

	_, err = writer.Write(data)
	if err != nil {
		h.logger.Error("failed to write file", zap.String("id", fileID), zap.Error(err))
		h.handleError(w, http.StatusInternalServerError, err)
		abort()
		return
	}
	writer.Close()
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	if err != nil {
		w.Write([]byte(err.Error()))
		h.logger.Error("failed to handle request", zap.Error(err))
	}
}
