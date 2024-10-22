package dist

import (
	"context"
	"fmt"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/scheduler"
)

type heartbeatService struct {
	scheduler   *scheduler.Scheduler
	runningJobs map[string]*scheduler.PendingJob
}

func (s *heartbeatService) Heartbeat(ctx context.Context, req *api.HeartbeatRequest) (*api.HeartbeatResponse, error) {

	for _, finishedJob := range req.FinishedJob {
		jobKey := fmt.Sprintf("%s%s", req.WorkerID, finishedJob.ID)
		if s.runningJobs[jobKey] != nil {
			s.scheduler.OnJobComplete(req.WorkerID, finishedJob.ID, &finishedJob)

			pendingJob := s.runningJobs[fmt.Sprintf("%s%s", req.WorkerID, finishedJob.ID)]

			pendingJob.Result.Stdout = finishedJob.Stdout
			pendingJob.Result.Stderr = finishedJob.Stderr
			pendingJob.Result.Error = finishedJob.Error
			pendingJob.Result.ExitCode = finishedJob.ExitCode

			close(s.runningJobs[jobKey].Finished)
			delete(s.runningJobs, jobKey)
		}
	}

	jobsToRun := make(map[build.ID]api.JobSpec)
	for i := 0; i < req.FreeSlots; i++ {
		pendingJob := s.scheduler.PickJob(ctx, req.WorkerID)
		if pendingJob == nil {
			break
		}

		jobsToRun[pendingJob.Job.ID] = *pendingJob.Job

		s.runningJobs[fmt.Sprintf("%s%s", req.WorkerID, pendingJob.Job.ID)] = pendingJob

	}

	return &api.HeartbeatResponse{
		JobsToRun: jobsToRun,
	}, nil
}
