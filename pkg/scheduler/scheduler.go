//go:build !solution

package scheduler

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
)

var TimeAfter = time.After

type PendingJob struct {
	Job      *api.JobSpec
	Finished chan struct{}
	Result   *api.JobResult
}

type Config struct {
	CacheTimeout time.Duration
	DepsTimeout  time.Duration
}

type Scheduler struct {
	logger         *zap.Logger
	config         Config
	queue          []*PendingJob
	queueMutex     sync.Mutex
	completed      map[build.ID]api.WorkerID
	completedMutex sync.Mutex
}

func NewScheduler(l *zap.Logger, config Config) *Scheduler {
	return &Scheduler{
		logger:    l,
		config:    config,
		queue:     make([]*PendingJob, 0),
		completed: map[build.ID]api.WorkerID{},
	}
}

func (c *Scheduler) LocateArtifact(id build.ID) (api.WorkerID, bool) {
	c.completedMutex.Lock()
	defer c.completedMutex.Unlock()

	workerID, ok := c.completed[id]
	if ok {
		c.logger.Info("Artifact was located", zap.String("id", id.String()), zap.String("worker", workerID.String()))
	} else {
		c.logger.Info("Artifact was not found", zap.String("id", id.String()))
	}
	return workerID, ok
}

func (c *Scheduler) OnJobComplete(workerID api.WorkerID, jobID build.ID, res *api.JobResult) bool {
	c.completedMutex.Lock()
	defer c.completedMutex.Unlock()

	c.logger.Info("job was completed", zap.String("workerID", workerID.String()), zap.String("jobID", jobID.String()))
	c.completed[jobID] = workerID
	return true
}

func (c *Scheduler) ScheduleJob(job *api.JobSpec) *PendingJob {
	pendingJob := &PendingJob{
		Job:      job,
		Finished: make(chan struct{}),
		Result:   &api.JobResult{},
	}
	c.queueMutex.Lock()
	defer c.queueMutex.Unlock()

	c.logger.Info("job was enqueued")
	c.queue = append(c.queue, pendingJob)
	return pendingJob
}

func (c *Scheduler) PickJob(ctx context.Context, workerID api.WorkerID) *PendingJob {
	if len(c.queue) == 0 {
		c.logger.Info("scheduler queue is empty")
		return nil
	}
	c.completedMutex.Lock()
	defer c.completedMutex.Unlock()

	pendingJob := c.queue[0]
	c.queue = c.queue[1:]
	c.logger.Info("picking job", zap.String("worker", workerID.String()), zap.String("jobID", pendingJob.Job.ID.String()))

	return pendingJob
}
