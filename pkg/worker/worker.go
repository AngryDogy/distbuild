//go:build !solution

package worker

import (
	"bytes"
	"context"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/artifact"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/filecache"
	"go.uber.org/zap"
	"net/http"
	"os"
	"os/exec"
	"sync"
)

type Worker struct {
	id                  api.WorkerID
	coordinatorEndpoint string
	logger              *zap.Logger
	fileCache           *filecache.Cache
	artifacts           *artifact.Cache
	heartbeatClient     *api.HeartbeatClient

	runningJobsMutex sync.Mutex
	runningJobs      []build.ID

	finishedJobsMutex sync.Mutex
	finishedJobs      []api.JobResult

	addedArtifacts []build.ID
	freeSlots      int
}

func New(
	workerID api.WorkerID,
	coordinatorEndpoint string,
	log *zap.Logger,
	fileCache *filecache.Cache,
	artifacts *artifact.Cache,
) *Worker {
	return &Worker{
		id:                  workerID,
		coordinatorEndpoint: coordinatorEndpoint,
		logger:              log,
		fileCache:           fileCache,
		artifacts:           artifacts,
		heartbeatClient:     api.NewHeartbeatClient(log, coordinatorEndpoint),
		runningJobs:         make([]build.ID, 0),
		finishedJobs:        make([]api.JobResult, 0),
		addedArtifacts:      make([]build.ID, 0),
		freeSlots:           1,
	}
}

func (w *Worker) ServeHTTP(rw http.ResponseWriter, r *http.Request) {

}

func (w *Worker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		response, err := w.heartbeatClient.Heartbeat(ctx, &api.HeartbeatRequest{
			WorkerID:       w.id,
			RunningJobs:    w.runningJobs,
			FreeSlots:      w.freeSlots,
			FinishedJob:    w.finishedJobs,
			AddedArtifacts: w.addedArtifacts,
		})

		if err != nil {
			w.logger.Error("heartbeat failed", zap.Error(err))
			continue
		}

		if len(response.JobsToRun) != 0 {
			w.logger.Info("jobs are received, started execution", zap.Any("response", response))
			var wg sync.WaitGroup
			for id, job := range response.JobsToRun {
				w.runningJobs = append(w.runningJobs, id)

				wg.Add(1)
				w.logger.Info("job started execution", zap.Any("job", job))
				go w.execute(&job, &wg)
			}
			wg.Wait()
		} else {
			//time.Sleep(time.Second)
		}
	}
}

func (w *Worker) execute(job *api.JobSpec, wg *sync.WaitGroup) {
	defer wg.Done()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	var err error

	for _, cmd := range job.Cmds {

		cmdExecutable := exec.Command(cmd.Exec[0], cmd.Exec[1:]...)
		cmdExecutable.Env = os.Environ()

		cmdExecutable.Stdout = &stdout
		cmdExecutable.Stderr = &stderr

		err = cmdExecutable.Run()
		if err != nil {
			w.logger.Error("cmd execution failed", zap.Error(err))
			break
		}
		w.logger.Info("cmd execution succeeded", zap.String("command", cmdExecutable.String()))
	}

	w.finishedJobsMutex.Lock()
	defer w.finishedJobsMutex.Unlock()

	w.finishedJobs = append(w.finishedJobs, api.JobResult{
		ID:     job.ID,
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
		Error: func() *string {
			s := stderr.String()
			return &s
		}(),
	})

	w.logger.Info("job finished", zap.Any("jobID", job.ID), zap.Any("stdout", stdout.String()), zap.Any("stderr", stderr.String()), zap.Any("error", err))

}
