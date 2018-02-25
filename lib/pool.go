package lib

import (
	"fmt"
	"sync"
	"time"
)

const (
	idleThreshold = 120 //seconds
)

//Runtime ...
type Runtime struct {
	ID         string
	Target     ProxyTarget
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
func (rp *RuntimePool) Index(key string) (*Runtime, bool) {
	rp.lock.RLock()
	defer rp.lock.RUnlock()

	r, ok := rp.pool[key]

	return r, ok
}

//Put ...
func (rp *RuntimePool) Put(key string, ID string, target ProxyTarget) (*Runtime, error) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	if _, ok := rp.pool[key]; ok {
		return nil, fmt.Errorf("%s existing", key)
	}

	r := &Runtime{
		ActiveTime: time.Now().Unix(),
		ID:         ID,
		Target:     target,
	}

	rp.pool[key] = r

	return r, nil
}

//Use ...
func (rp *RuntimePool) Use(key string) (*Runtime, error) {
	rp.lock.Lock()
	defer rp.lock.Unlock()

	r, ok := rp.pool[key]
	if ok {
		//Update active time
		r.ActiveTime = time.Now().Unix()
		return r, nil
	}

	return r, fmt.Errorf("%s not existing", key)
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
