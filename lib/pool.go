package lib

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	idleThreshold = 60 //seconds
)

//Runtime ...
type Runtime struct {
	ID         string
	ActiveTime int64
}

//RuntimePool ...
type RuntimePool struct {
	pool map[string]*Runtime
	lock *sync.RWMutex
}

//NewRuntimePool ...
func NewRuntimePool() *RuntimePool {
	return &RuntimePool{
		pool: make(map[string]*Runtime),
		lock: new(sync.RWMutex),
	}
}

//Index ...
func (rp *RuntimePool) Index(prefix, ID string) (*Runtime, bool) {
	rp.lock.RLock()
	defer rp.lock.RUnlock()

	r, ok := rp.pool[giveMeKey(prefix, ID)]

	return r, ok
}

//Use ...
func (rp *RuntimePool) Use(prefix, ID string) (*Runtime, error) {
	if len(ID) == 0 {
		return nil, errors.New("empty ID")
	}
	rp.lock.Lock()
	defer rp.lock.Unlock()

	r, ok := rp.pool[giveMeKey(prefix, ID)]
	if ok {
		//Update active time
		r.ActiveTime = time.Now().Unix()
		return r, nil
	}

	r = &Runtime{
		ActiveTime: time.Now().Unix(),
		ID:         ID,
	}

	rp.pool[giveMeKey(prefix, ID)] = r

	return r, nil
}

//Remove ...
func (rp *RuntimePool) Remove(prefix, ID string) error {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	if _, ok := rp.pool[giveMeKey(prefix, ID)]; ok {
		delete(rp.pool, giveMeKey(prefix, ID))
	}

	return nil
}

//Garbages ...
func (rp *RuntimePool) Garbages() []*Runtime {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	var garbages []*Runtime
	now := time.Now().Unix()
	for k, v := range rp.pool {
		if now >= v.ActiveTime+idleThreshold {
			//Garbage
			garbages = append(garbages, v)
			delete(rp.pool, k)
		}
	}

	return garbages
}

func giveMeKey(prefix, ID string) string {
	return fmt.Sprintf("%s:%s", prefix, ID)
}
