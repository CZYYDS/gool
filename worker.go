package gool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

var nowFunc = time.Now()

type Worker struct {
	pool      *ThreadPool //所属线程池
	close     bool        //是否关闭
	ctx       context.Context
	createdAt time.Time
	returnAt  *time.Time
	mutex     sync.Mutex
}

func NewWorker(ctx context.Context, pool *ThreadPool) *Worker {
	return &Worker{
		createdAt: nowFunc,
		ctx:       ctx,
		mutex:     sync.Mutex{},
		returnAt:  nil,
		pool:      pool,
	}
}
func (w *Worker) Close() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if !w.close {
		w.close = true
	}
}
func (w *Worker) Expire(maxLifeTime, maxIdleTime time.Duration) bool {
	if maxLifeTime > 0 {
		return nowFunc.After((w.createdAt).Add(maxLifeTime))
	}
	if maxIdleTime > 0 {
		if w.returnAt != nil {
			return nowFunc.After((*w.returnAt).Add(maxIdleTime))
		}
	}
	return false
}

// 运行一个协程
func (w *Worker) Run() {
	if w.close {
		return
	}
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
		}
		//判断Worker是否过期
		if w.Expire(w.pool.MaxLifeTime, w.pool.MaxIdleTime) {
			w.Close()
			w.pool.mayOpenWorkerThread()
			return
		}
		select {
		case <-w.ctx.Done():
			return
		case task := <-w.pool.RequestTasks:
			//如果收到的task为nil，说明强制关闭协程池
			if task != nil {
				task()
				w.returnAt = &nowFunc
				atomic.AddInt64(&w.pool.finishedTaskNum, 1)
				atomic.AddInt64(&w.pool.WaitTaskNum, -1)
				continue
			} else {
				w.Close()
				return
			}
		default:
		}
		//走到这说明worker是空闲的
		w.pool.mutex.Lock()
		w.returnAt = &nowFunc
		w.pool.IdleWorkers = append(w.pool.IdleWorkers, w)
		atomic.AddInt64(&w.pool.IdleWorkersNum, 1)
		w.pool.mutex.Unlock()
		return
	}

}
