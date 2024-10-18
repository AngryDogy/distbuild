//go:build !solution

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"io"
	"net/http"
)

type HeartbeatClient struct {
	logger   *zap.Logger
	endpoint string
}

func NewHeartbeatClient(l *zap.Logger, endpoint string) *HeartbeatClient {
	return &HeartbeatClient{
		logger:   l,
		endpoint: endpoint,
	}
}

func (c *HeartbeatClient) Heartbeat(ctx context.Context, req *HeartbeatRequest) (*HeartbeatResponse, error) {
	requestData, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	serverURL := fmt.Sprintf("%s/heartbeat", c.endpoint)
	response, err := http.Post(serverURL, "application/json", bytes.NewBuffer(requestData))
	if err != nil {
		c.logger.Error("failed to make post /heartbeat request", zap.Error(err))
		return nil, err
	}

	responseData, err := io.ReadAll(response.Body)
	c.logger.Info("post /heartbeat response: ", zap.String("response", string(responseData)))
	if err != nil {
		c.logger.Error("failed to read response body", zap.Error(err))
		return nil, err
	}
	defer response.Body.Close()

	var heartbeatResponse HeartbeatResponse
	err = json.Unmarshal(responseData, &heartbeatResponse)
	if err != nil {
		c.logger.Error("failed to unmarshal response body to heartbeat response", zap.Error(err))
		return nil, &heartbeatError{
			error: string(responseData),
		}
	}

	return &heartbeatResponse, nil
}

type heartbeatError struct {
	error string
}

func (e *heartbeatError) Error() string {
	return e.error
}
