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

	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
)

type BuildClient struct {
	logger   *zap.Logger
	endpoint string
}

func NewBuildClient(l *zap.Logger, endpoint string) *BuildClient {
	return &BuildClient{
		logger:   l,
		endpoint: endpoint,
	}
}

func (c *BuildClient) StartBuild(ctx context.Context, request *BuildRequest) (*BuildStarted, StatusReader, error) {
	serverURL := fmt.Sprintf("%s/build", c.endpoint)

	requestData, err := json.Marshal(request)
	if err != nil {
		c.logger.Error("failed to marshal build request", zap.Error(err))
		return nil, nil, err
	}

	response, err := http.Post(serverURL, "application/json", bytes.NewBuffer(requestData))
	if err != nil {
		c.logger.Error("failed to make post /build request", zap.Error(err))
		return nil, nil, err
	}

	if response.StatusCode != http.StatusOK {
		c.logger.Info("response does not have ok status")
		data, err := io.ReadAll(response.Body)
		if err != nil {
			return nil, nil, err
		}
		defer response.Body.Close()

		return nil, nil, &buildError{
			error: string(data),
		}
	}

	decoder := json.NewDecoder(response.Body)
	var started BuildStarted
	err = decoder.Decode(&started)
	if err != nil {
		c.logger.Error("failed to decode response body to buildStarted", zap.Error(err))
		return nil, nil, err
	}

	c.logger.Info("decoded buildStarted successfully")
	return &started, newStatusReader(c.logger, response.Body), nil
}

func (c *BuildClient) SignalBuild(ctx context.Context, buildID build.ID, signal *SignalRequest) (*SignalResponse, error) {
	serverURL := fmt.Sprintf("%s/signal?build_id=%s", c.endpoint, buildID.String())

	requestData, err := json.Marshal(signal)
	if err != nil {
		c.logger.Error("failed to marshal signal request", zap.Error(err))
		return nil, err
	}

	response, err := http.Post(serverURL, "application/json", bytes.NewBuffer(requestData))
	if err != nil {
		c.logger.Error("failed to make post /signal request", zap.Error(err))
		return nil, err
	}

	responseData, err := io.ReadAll(response.Body)
	if err != nil {
		c.logger.Error("failed to read response body", zap.Error(err))
		return nil, err
	}
	defer response.Body.Close()

	var signalResponse SignalResponse
	err = json.Unmarshal(responseData, &signalResponse)
	if err != nil {
		c.logger.Error("failed to decode response body to signalResponse", zap.Error(err))
		return nil, &buildError{
			error: string(responseData),
		}
	}

	return &signalResponse, nil
}

type buildError struct {
	error string
}

func (e *buildError) Error() string {
	return e.error
}
