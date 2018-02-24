package lib

import (
	"context"
	"log"
	"time"
)

//ProxyTarget ...
type ProxyTarget string

//SchedulerConfig ...
type SchedulerConfig struct {
	DockerHost string
	HPort      int32
	Harbor     string
}

//Scheduler ...
type Scheduler struct {
	pool     *RuntimePool
	executor *Executor
	packer   *Packer
	ctx      context.Context
}

//SchedulePolicy ...
type SchedulePolicy struct {
	Registry string //Where to retrieve the image
	Namepace string //Project or org
	Image    string
	Tag      string
}

//NewScheduler ...
func NewScheduler(ctx context.Context, cfg SchedulerConfig) *Scheduler {
	return &Scheduler{
		pool:     NewRuntimePool(),
		executor: NewExecutor(cfg.DockerHost, cfg.HPort),
		packer:   NewPacker(cfg.DockerHost, cfg.HPort, cfg.Harbor),
		ctx:      ctx,
	}
}

//Start ...
func (s *Scheduler) Start() {
	go func() {
		tk := time.Tick(30 * time.Second)
		for {
			select {
			case <-tk:
				//Garbage collection
				garbages := s.pool.Garbages()
				if len(garbages) > 0 {
					for _, v := range garbages {
						//Clear
						if err := s.executor.Destroy(v.ID); err != nil {
							log.Fatalf("garbage collection %s error: %s\n", v.ID, err)
						}
					}
				}
			case <-s.ctx.Done():
				log.Println("Scheduler stop")
				return
			}
		}
	}()
}

//Schedule ...
func (s *Scheduler) Schedule() ProxyTarget {
	return ""
}

//
//func (s *Scheduler)
