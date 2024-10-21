package dist

import (
	"context"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/scheduler"
)

type heartbeatService struct {
	scheduler   *scheduler.Scheduler
	runningJobs map[build.ID]*scheduler.PendingJob
}

func (s *heartbeatService) Heartbeat(ctx context.Context, req *api.HeartbeatRequest) (*api.HeartbeatResponse, error) {

	jobsToRun := make(map[build.ID]api.JobSpec)
	for i := 0; i < req.FreeSlots; i++ {
		pendingJob := s.scheduler.PickJob(ctx, req.WorkerID)
		if pendingJob != nil {
			jobsToRun[pendingJob.Job.ID] = *pendingJob.Job
			s.runningJobs[pendingJob.Job.ID] = pendingJob
		}
	}

	for _, finishedJob := range req.FinishedJob {
		if s.runningJobs[finishedJob.ID] != nil {

			pendingJob := s.runningJobs[finishedJob.ID]

			pendingJob.Result.Stdout = finishedJob.Stdout
			pendingJob.Result.Stderr = finishedJob.Stderr
			pendingJob.Result.Error = finishedJob.Error
			pendingJob.Result.ExitCode = finishedJob.ExitCode

			close(s.runningJobs[finishedJob.ID].Finished)
			delete(s.runningJobs, finishedJob.ID)
		}
	}

	return &api.HeartbeatResponse{
		JobsToRun: jobsToRun,
	}, nil
}
