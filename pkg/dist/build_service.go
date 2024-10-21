package dist

import (
	"context"
	"errors"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/filecache"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/scheduler"
	"sync"
)

type buildService struct {
	fileCache *filecache.Cache
	scheduler *scheduler.Scheduler
	signalMap map[build.ID]chan struct{}
}

func (s *buildService) StartBuild(ctx context.Context, request *api.BuildRequest, w api.StatusWriter) error {

	buildID := build.NewID()
	missingFiles := make([]build.ID, 0)
	for id := range request.Graph.SourceFiles {
		_, _, err := s.fileCache.Get(id)
		if err != nil {
			missingFiles = append(missingFiles, id)
		}
	}

	err := w.Started(&api.BuildStarted{
		ID:           buildID,
		MissingFiles: missingFiles,
	})
	if err != nil {
		return err
	}

	s.signalMap[buildID] = make(chan struct{}, 1)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.signalMap[buildID]:
	}

	var wg sync.WaitGroup
	for _, job := range request.Graph.Jobs {
		pendingJob := s.scheduler.ScheduleJob(&api.JobSpec{
			Job:         job,
			SourceFiles: request.Graph.SourceFiles,
			Artifacts:   make(map[build.ID]api.WorkerID),
		})

		wg.Add(1)
		go func(currentJob *scheduler.PendingJob) {
			defer wg.Done()
			select {
			case <-currentJob.Finished:
				w.Updated(&api.StatusUpdate{
					JobFinished: &api.JobResult{
						ID:       currentJob.Job.ID,
						Stdout:   currentJob.Result.Stdout,
						Stderr:   currentJob.Result.Stderr,
						Error:    currentJob.Result.Error,
						ExitCode: currentJob.Result.ExitCode,
					},
				})
			case <-ctx.Done():
			}
		}(pendingJob)
	}
	wg.Wait()
	return nil
}

func (s *buildService) SignalBuild(ctx context.Context, buildID build.ID, signal *api.SignalRequest) (*api.SignalResponse, error) {
	if s.signalMap[buildID] == nil {
		return nil, errors.New("build not found")
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case s.signalMap[buildID] <- struct{}{}:
		return &api.SignalResponse{}, nil
	default:
		return nil, errors.New("build already signaled")
	}
}
