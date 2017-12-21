package pool

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/henrylee2cn/goutil/coarsetime"
)

type (
	// Worker woker interface
	Worker interface {
		Health() bool
		Close() error
	}
	// Workshop working workshop
	Workshop struct {
		newFn           func() (Worker, error)
		maxQuota        int
		maxIdleDuration time.Duration
		infos           map[*workerInfo]struct{}
		mostFree        *workerInfo
		stats           *WorkshopStats
		lock            sync.Mutex
		wg              sync.WaitGroup
		closeCh         chan struct{}
	}
	workerInfo struct {
		worker     Worker
		jobNum     int32
		idleExpire time.Time
		wg         sync.WaitGroup
	}
	// WorkshopStats workshop stats
	WorkshopStats struct {
		Worker    int32
		Idle      int32
		Created   uint64
		Closed    uint64
		Hire      uint64
		Fire      uint64
		Doing     int32
		MostUsed  int32
		LeastUsed int32
	}
)

const (
	defaultWorkerMaxQuota        = 64
	defaultWorkerMaxIdleDuration = 3 * time.Minute
)

// NewWorkshop creates a new workshop.
// If maxQuota<=0, will use default value.
// If maxIdleDuration<=0, will use default value.
func NewWorkshop(maxQuota int, maxIdleDuration time.Duration, newWorkerFunc func() (Worker, error)) *Workshop {
	if maxQuota <= 0 {
		maxQuota = defaultWorkerMaxQuota
	}
	if maxIdleDuration <= 0 {
		maxIdleDuration = defaultWorkerMaxIdleDuration
	}
	w := new(Workshop)
	w.stats = new(WorkshopStats)
	w.maxQuota = maxQuota
	w.maxIdleDuration = maxIdleDuration
	w.infos = make(map[*workerInfo]struct{}, maxQuota)
	w.closeCh = make(chan struct{})
	w.newFn = func() (worker Worker, err error) {
		defer func() {
			if p := recover(); p != nil {
				err = fmt.Errorf("%v", p)
			} else {
				w.stats.Created++
			}
		}()
		return newWorkerFunc()
	}
	return w
}

// Do assign a worker to execute the callback function.
func (w *Workshop) Do(callback func(Worker) error) error {
	select {
	case <-w.closeCh:
		return ErrWorkshopClosed
	default:
	}
	w.lock.Lock()
	info, err := w.hireLocked()
	w.lock.Unlock()
	if err != nil {
		return err
	}
	worker := info.worker
	defer func() {
		w.lock.Lock()
		w.fireLocked(info)
		w.lock.Unlock()
	}()
	return callback(worker)
}

// ErrWorkshopClosed error: 'workshop is closed'
var ErrWorkshopClosed = fmt.Errorf("%s", "workshop is closed")

func (w *Workshop) hireLocked() (*workerInfo, error) {
	var info *workerInfo
GET:
	info = w.mostFree
	if len(w.infos) >= w.maxQuota || (info != nil && info.jobNum == 0) {
		if info.jobNum == 0 {
			w.stats.Idle--
			if coarsetime.FloorTimeNow().After(info.idleExpire) {
				delete(w.infos, info)
				info.worker.Close()
				w.stats.Closed++
				w.updateFreeLocked()
				goto GET
			}
		}
		info.use()
		w.updateFreeLocked()
	} else {
		worker, err := w.newFn()
		if err != nil {
			return nil, err
		}
		info = &workerInfo{
			worker: worker,
			wg:     w.wg,
		}
		info.use()
		w.infos[info] = struct{}{}
		w.mostFree = info
	}

	w.stats.Hire++
	return info, nil
}

func (info *workerInfo) use() {
	info.jobNum++
	info.wg.Add(1)
}

func (info *workerInfo) free() {
	info.jobNum--
	info.wg.Add(-1)
}

func (w *Workshop) fireLocked(info *workerInfo) {
	w.stats.Fire++
	if !info.worker.Health() {
		delete(w.infos, info)
		w.stats.Closed++
		return
	}
	info.free()
	jobNum := info.jobNum
	if jobNum == 0 {
		info.idleExpire = coarsetime.CeilingTimeNow().Add(w.maxIdleDuration)
		w.stats.Idle++
	}
	if jobNum >= w.mostFree.jobNum {
		return
	}
	w.mostFree = info
}

func (w *Workshop) updateFreeLocked() {
	if len(w.infos) == 0 {
		w.mostFree = nil
		return
	}
	var mostFree *workerInfo
	for info := range w.infos {
		if mostFree != nil && info.jobNum >= mostFree.jobNum {
			continue
		}
		mostFree = info
	}
	w.mostFree = mostFree
}

// Stats returns the current workshop stats.
// Remove the overtime idle workers.
func (w *Workshop) Stats() WorkshopStats {
	w.lock.Lock()
	w.stats.Doing = int32(w.stats.Hire - w.stats.Fire)
	var max, min int32
	var tmp int32
	min = math.MaxInt32
	var shouldUpdate bool
	for info := range w.infos {
		if info.jobNum == 0 && coarsetime.FloorTimeNow().After(info.idleExpire) {
			delete(w.infos, info)
			info.worker.Close()
			w.stats.Closed++
			w.stats.Idle--
			shouldUpdate = true
			continue
		}
		tmp = info.jobNum
		if tmp > max {
			max = tmp
		}
		if tmp < min {
			min = tmp
		}
	}
	if shouldUpdate {
		w.updateFreeLocked()
	}
	w.stats.Worker = int32(len(w.infos))
	if min == math.MaxInt32 {
		min = 0
	}
	w.stats.LeastUsed = min
	w.stats.MostUsed = max
	stats := *w.stats
	w.lock.Unlock()
	return stats
}

// Close wait for all the work to be completed and close the workshop.
func (w *Workshop) Close() {
	select {
	case <-w.closeCh:
		return
	default:
		close(w.closeCh)
	}
	w.wg.Wait()
	w.lock.Lock()
	defer w.lock.Unlock()
	for info := range w.infos {
		info.worker.Close()
		w.stats.Closed++
	}
	w.infos = nil
	w.stats.Idle = 0
}