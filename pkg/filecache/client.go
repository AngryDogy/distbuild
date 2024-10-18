//go:build !solution

package filecache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"go.uber.org/zap"

	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
)

type Client struct {
	logger   *zap.Logger
	endpoint string
}

func NewClient(l *zap.Logger, endpoint string) *Client {
	return &Client{
		logger:   l,
		endpoint: endpoint,
	}
}

func (c *Client) Upload(ctx context.Context, id build.ID, localPath string) error {
	serverURL := fmt.Sprintf("%s/file?id=%s", c.endpoint, id.String())

	file, err := os.Open(localPath)
	if err != nil {
		c.logger.Error("failed to open file", zap.String("path", localPath), zap.Error(err))
		return err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		c.logger.Error("failed to read file", zap.String("path", localPath), zap.Error(err))
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serverURL, bytes.NewReader(data))
	if err != nil {
		c.logger.Error("failed to build upload request", zap.String("url", serverURL), zap.Error(err))
		return err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Error("failed to do request", zap.String("url", serverURL), zap.Error(err))
		return err
	}

	if response.StatusCode != http.StatusOK {
		return errors.New("failed to upload file")
	}

	return nil
}

func (c *Client) Download(ctx context.Context, localCache *Cache, id build.ID) error {
	serverURL := fmt.Sprintf("%s/file?id=%s", c.endpoint, id.String())

	req, err := http.NewRequestWithContext(ctx, "GET", serverURL, nil)
	if err != nil {
		c.logger.Error("failed to build download request", zap.String("url", serverURL), zap.Error(err))
		return err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		c.logger.Error("failed to do request", zap.String("url", serverURL), zap.Error(err))
		return err
	}
	defer response.Body.Close()

	writer, abort, err := localCache.Write(id)
	if err != nil {
		c.logger.Error("failed to create writer", zap.Error(err))
		return err
	}

	_, err = io.Copy(writer, response.Body)
	if err != nil {
		c.logger.Error("failed to write file", zap.String("url", serverURL), zap.Error(err))
		abort()
		return err
	}
	return writer.Close()
}
