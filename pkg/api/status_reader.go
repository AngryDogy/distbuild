package api

import (
	"encoding/json"
	"go.uber.org/zap"
	"io"
)

type StatusReader interface {
	Close() error
	Next() (*StatusUpdate, error)
}

type statusReader struct {
	logger    *zap.Logger
	decoder   *json.Decoder
	closeFunc func() error
}

func newStatusReader(l *zap.Logger, r io.ReadCloser) StatusReader {
	return &statusReader{
		logger:  l,
		decoder: json.NewDecoder(r),
		closeFunc: func() error {
			return r.Close()
		},
	}
}

func (s *statusReader) Close() error {
	return s.closeFunc()
}

func (s *statusReader) Next() (*StatusUpdate, error) {
	var update StatusUpdate
	err := s.decoder.Decode(&update)
	if err != nil {
		return nil, err
	}

	s.logger.Info("got next updated status successfully")
	return &update, nil
}
