//go:build !solution

package client

import (
	"context"
	"errors"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/filecache"
	"io"
	"log"
	"path/filepath"

	"go.uber.org/zap"

	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
)

type Client struct {
	logger      *zap.Logger
	apiEndpoint string
	sourceDir   string
	cacheFile   *filecache.Cache
	cacheClient *filecache.Client
	buildClient *api.BuildClient
}

func NewClient(
	l *zap.Logger,
	apiEndpoint string,
	sourceDir string,
) *Client {
	return &Client{
		logger:      l,
		apiEndpoint: apiEndpoint,
		sourceDir:   sourceDir,
		cacheFile: func() *filecache.Cache {
			cache, err := filecache.New(sourceDir)
			if err != nil {
				log.Fatal("Failed to create cache", zap.Error(err))
			}
			return cache
		}(),
		cacheClient: filecache.NewClient(l, apiEndpoint),
		buildClient: api.NewBuildClient(l, apiEndpoint),
	}
}

type BuildListener interface {
	OnJobStdout(jobID build.ID, stdout []byte) error
	OnJobStderr(jobID build.ID, stderr []byte) error

	OnJobFinished(jobID build.ID) error
	OnJobFailed(jobID build.ID, code int, error string) error
}

func (c *Client) Build(ctx context.Context, graph build.Graph, lsn BuildListener) error {
	startStatusBuild, statusReader, err := c.buildClient.StartBuild(ctx, &api.BuildRequest{Graph: graph})
	if err != nil {
		c.logger.Error("failed to start build", zap.Error(err))
		return err
	}

	for _, id := range startStatusBuild.MissingFiles {
		q := id.Path()
		_ = q
		pathToFile := filepath.Join(c.sourceDir, graph.SourceFiles[id])

		err = c.cacheClient.Upload(ctx, id, pathToFile)
		if err != nil {
			c.logger.Error("failed to upload file", zap.Error(err))
			return err
		}
	}

	_, err = c.buildClient.SignalBuild(ctx, startStatusBuild.ID, &api.SignalRequest{})
	if err != nil {
		c.logger.Error("failed to signal build", zap.String("buildID", startStatusBuild.ID.String()), zap.Error(err))
		return err
	}

	for {
		statusUpdate, err := statusReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}

		// Should never happen
		if err != nil {
			c.logger.Error("failed to read build status, unexpected error", zap.String("buildID", startStatusBuild.ID.String()), zap.Error(err))
			return err
		}

		if statusUpdate.JobFinished != nil {
			c.logger.Info("job finished", zap.String("jobId", statusUpdate.JobFinished.ID.String()))
			lsn.OnJobStdout(statusUpdate.JobFinished.ID, statusUpdate.JobFinished.Stdout)
			lsn.OnJobStderr(statusUpdate.JobFinished.ID, statusUpdate.JobFinished.Stderr)

			if statusUpdate.JobFinished.Error != nil {
				lsn.OnJobFinished(statusUpdate.JobFinished.ID)
			} else {
				lsn.OnJobFailed(statusUpdate.JobFinished.ID, statusUpdate.JobFinished.ExitCode, *statusUpdate.JobFinished.Error)
			}
		}

		if statusUpdate.BuildFailed != nil {
			c.logger.Info("build failed", zap.String("error", statusUpdate.BuildFailed.Error))
			return errors.New(statusUpdate.BuildFailed.Error)
		}
	}

	return nil
}
