//go:build !solution

package dist

import (
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/api"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/build"
	"net/http"
	"time"

	"go.uber.org/zap"

	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/filecache"
	"gitlab.com/manytask/itmo-go/public/distbuild/pkg/scheduler"
)

type Coordinator struct {
	logger           *zap.Logger
	mux              *http.ServeMux
	fileCache        *filecache.Cache
	fileCacheClient  *filecache.Client
	heartbeatHandler *api.HeartbeatHandler
	buildHandler     *api.BuildHandler
	fileCacheHandler *filecache.Handler
	scheduler        *scheduler.Scheduler
}

var defaultConfig = scheduler.Config{
	CacheTimeout: time.Millisecond * 10,
	DepsTimeout:  time.Millisecond * 100,
}

func NewCoordinator(
	log *zap.Logger,
	fileCache *filecache.Cache,
) *Coordinator {

	coordinatorScheduler := scheduler.NewScheduler(log, defaultConfig)

	coordinator := &Coordinator{
		logger: log,
		mux:    http.NewServeMux(),

		fileCache:        fileCache,
		fileCacheHandler: filecache.NewHandler(log, fileCache),

		heartbeatHandler: api.NewHeartbeatHandler(log, &heartbeatService{
			scheduler:   coordinatorScheduler,
			runningJobs: make(map[string]*scheduler.PendingJob),
		}),
		buildHandler: api.NewBuildService(log, &buildService{
			fileCache: fileCache,
			scheduler: coordinatorScheduler,
			signalMap: make(map[build.ID]chan struct{}),
		}),
		scheduler: coordinatorScheduler,
	}

	coordinator.heartbeatHandler.Register(coordinator.mux)
	coordinator.buildHandler.Register(coordinator.mux)
	coordinator.fileCacheHandler.Register(coordinator.mux)

	coordinator.mux.HandleFunc("/artifact", coordinator.getArtifactWorker)

	return coordinator

}

func (c *Coordinator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}

func (c *Coordinator) getArtifactWorker(w http.ResponseWriter, r *http.Request) {
	artifactID := r.URL.Query().Get("id")
	if artifactID == "" {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("artifact id is required"))
		return
	}

	var id build.ID
	err := id.UnmarshalText([]byte(artifactID))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	workerID, ok := c.scheduler.LocateArtifact(id)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	c.logger.Info("Artifact was successfully found", zap.String("artifactID", artifactID), zap.Any("workerID", workerID))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(workerID.String()))
}
