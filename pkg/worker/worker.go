//go:build !solution

package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/artifact"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/filecache"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	freeWorkerSlots = 3
)

type Worker struct {
	id                  api.WorkerID
	logger              *zap.Logger
	coordinatorEndpoint string
	mux                 *http.ServeMux

	fileCache  *filecache.Cache
	fileClient *filecache.Client

	artifacts *artifact.Cache

	heartbeatClient *api.HeartbeatClient

	runningJobsMutex sync.Mutex
	runningJobs      map[build.ID]bool

	finishedJobsMutex sync.Mutex
	finishedJobs      []api.JobResult

	addedArtifacts []build.ID

	freeSlots *atomic.Int64

	workdir string

	ticker *time.Ticker
}

func New(
	workerID api.WorkerID,
	coordinatorEndpoint string,
	log *zap.Logger,
	fileCache *filecache.Cache,
	artifacts *artifact.Cache,
) *Worker {
	mux := http.NewServeMux()

	workerIdSplit := strings.Split(workerID.String(), "/")
	workerName := fmt.Sprintf("%s%s", workerIdSplit[len(workerIdSplit)-2], workerIdSplit[len(workerIdSplit)-1])
	cachePath := fileCache.GetCacheDir()

	var workerPath strings.Builder
	for _, str := range strings.Split(cachePath, "/") {
		workerPath.WriteString(str + "/")
		if str == workerName {
			break
		}
	}

	return &Worker{
		id:                  workerID,
		coordinatorEndpoint: coordinatorEndpoint,
		logger:              log,
		mux:                 mux,

		fileCache:  fileCache,
		fileClient: filecache.NewClient(log, coordinatorEndpoint),

		artifacts: artifacts,

		heartbeatClient: api.NewHeartbeatClient(log, coordinatorEndpoint),

		runningJobs:    map[build.ID]bool{},
		finishedJobs:   make([]api.JobResult, 0),
		addedArtifacts: make([]build.ID, 0),
		freeSlots: func() *atomic.Int64 {
			freeSlots := atomic.Int64{}
			freeSlots.Store(freeWorkerSlots)
			return &freeSlots
		}(),

		workdir: workerPath.String(),

		ticker: time.NewTicker(time.Second),
	}
}

func (w *Worker) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	w.mux.ServeHTTP(rw, r)
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		response, err := w.heartbeat(ctx)

		if err != nil {
			w.logger.Error("heartbeat failed", zap.Error(err))
			continue
		}

		if len(response.JobsToRun) != 0 {
			w.logger.Info("jobs are received, started execution", zap.Any("response", response))
			for id, job := range response.JobsToRun {
				w.runningJobsMutex.Lock()
				w.runningJobs[id] = true
				w.runningJobsMutex.Unlock()

				w.freeSlots.Add(-1)

				w.logger.Info("job started execution", zap.Any("job", job))
				go w.execute(ctx, &job)
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.ticker.C:
		}

	}
}

func (w *Worker) heartbeat(ctx context.Context) (*api.HeartbeatResponse, error) {
	w.runningJobsMutex.Lock()
	defer w.runningJobsMutex.Unlock()

	currentRunningJobs := make([]build.ID, 0)
	for id, _ := range w.runningJobs {
		currentRunningJobs = append(currentRunningJobs, id)
	}

	w.finishedJobsMutex.Lock()
	defer w.finishedJobsMutex.Unlock()

	response, err := w.heartbeatClient.Heartbeat(ctx, &api.HeartbeatRequest{
		WorkerID:       w.id,
		RunningJobs:    currentRunningJobs,
		FreeSlots:      int(w.freeSlots.Load()),
		FinishedJob:    w.finishedJobs,
		AddedArtifacts: w.addedArtifacts,
	})

	w.finishedJobs = make([]api.JobResult, 0)

	return response, err

}

func (w *Worker) execute(ctx context.Context, job *api.JobSpec) {
	var err error
	err = w.downloadFiles(ctx, job.SourceFiles)
	if err != nil {
		w.logger.Error("download files failed", zap.Error(err))
		return
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	for _, cmdToRender := range job.Cmds {

		cmd, err := cmdToRender.Render(build.JobContext{
			SourceDir: w.workdir,
			OutputDir: w.workdir,
		})
		if err != nil {
			w.logger.Error("render cmd failed", zap.Error(err))
			continue
		}

		if cmd.Exec != nil {

			var cmdExecutable *exec.Cmd
			if len(cmd.Exec) == 1 {
				cmdExecutable = exec.Command(cmd.Exec[0])
			} else {
				cmdExecutable = exec.Command(cmd.Exec[0], cmd.Exec[1:]...)
			}

			cmdExecutable.Env = os.Environ()

			cmdExecutable.Stdout = &stdout
			cmdExecutable.Stderr = &stderr

			cmdExecutable.Dir = w.workdir

			err = cmdExecutable.Run()
			if err != nil {
				w.logger.Error("cmd execution failed", zap.Error(err))
				break
			}
			w.logger.Info("cmd execution succeeded", zap.String("command", cmdExecutable.String()))
		}

		if cmd.CatOutput != "" {
			//w.fileCache.

		}

	}

	w.runningJobsMutex.Lock()
	w.finishedJobsMutex.Lock()
	defer w.runningJobsMutex.Unlock()
	defer w.finishedJobsMutex.Unlock()

	delete(w.runningJobs, job.ID)

	w.freeSlots.Add(1)

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

func (w *Worker) downloadFiles(ctx context.Context, sourcesFiles map[build.ID]string) error {
	for id, name := range sourcesFiles {
		path, _, err := w.fileCache.Get(id)
		if err != nil {
			if errors.Is(err, filecache.ErrNotFound) {
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
					err := w.fileClient.Download(ctx, w.fileCache, id)
					if err == nil {
						break
					}
					w.logger.Error("download failed", zap.Error(err), zap.String("fileID", id.String()))
				}
			} else {
				return err
			}
		}

		data, err := os.ReadFile(path)
		_ = data
		if err != nil {
			return err
		}

		var newPath strings.Builder
		newPath.WriteString(w.workdir)

		splitName := strings.Split(name, "/")
		for i := 0; i < len(splitName)-1; i++ {
			newPath.WriteString(splitName[i])
			newPath.WriteByte('/')
		}

		err = os.MkdirAll(newPath.String(), os.ModePerm)
		if err != nil {
			return err
		}

		var writer io.WriteCloser
		for {
			writer, err = os.Create(fmt.Sprintf("%s/%s", newPath.String(), splitName[len(splitName)-1]))
			if err != nil {
				if errors.Is(err, filecache.ErrExists) {
					os.Remove(path)
					continue
				}
				return err
			}
			break
		}
		defer writer.Close()

		_, err = writer.Write(data)
		if err != nil {
			return err
		}

	}
	return nil
}
