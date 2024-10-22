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
		logger:    log,
		mux:       http.NewServeMux(),
		fileCache: fileCache,
		heartbeatHandler: api.NewHeartbeatHandler(log, &heartbeatService{
			scheduler:   coordinatorScheduler,
			runningJobs: make(map[build.ID]*scheduler.PendingJob),
		}),
		buildHandler: api.NewBuildService(log, &buildService{
			fileCache: fileCache,
			scheduler: coordinatorScheduler,
			signalMap: make(map[build.ID]chan struct{}),
		}),
		fileCacheHandler: filecache.NewHandler(log, fileCache),
	}
	coordinator.heartbeatHandler.Register(coordinator.mux)
	coordinator.buildHandler.Register(coordinator.mux)
	coordinator.fileCacheHandler.Register(coordinator.mux)

	return coordinator

}

func (c *Coordinator) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mux.ServeHTTP(w, r)
}
