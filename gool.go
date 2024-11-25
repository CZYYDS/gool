package gool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultIdleTimeout = 10 * time.Second
	defaultMaxCapacity = 10
	defaultMaxWorkers  = 8
)

type ThreadPool struct {
	mutex        sync.Mutex    //锁
	RequestTasks chan func()   //任务队列
	openerChan   chan struct{} //Work线程创建信号
	cleanerChan  *struct{}     //Work线程清除信号
	IdleWorkers  []*Worker     //空闲Worker列表

	IdleWorkersNum int64 //空闲Worker数量

	MaxWorkers  int64 //最大工作线程数
	OpenWorkers int64 //当前工作线程数
	minWorkers  int64 //最小工作线程数

	TaskId      int64 //任务id,线程安全的自生成任务Id
	maxCapacity int64 //最大任务数

	MaxIdleTime time.Duration //最大空闲时间
	MaxLifeTime time.Duration //最大存活时间

	finishedTaskNum int64 //已完成的任务数
	WaitTaskNum     int64 //等待执行的任务数

	waitGroup sync.WaitGroup

	stopped atomic.Bool //是否关闭

	ctx    context.Context
	cancel context.CancelFunc
}

func (pool *ThreadPool) getOpenWorkers() int64 {
	return atomic.LoadInt64(&pool.OpenWorkers)
}
func (pool *ThreadPool) getWaitTaskNum() int64 {
	return atomic.LoadInt64(&pool.WaitTaskNum)
}
func (pool *ThreadPool) getIdleWorkersNum() int64 {
	return atomic.LoadInt64(&pool.IdleWorkersNum)
}
func New(maxCapacity, maxWorkers int64, options ...Option) *ThreadPool {

	// Instantiate the pool
	ctx, cancel := context.WithCancel(context.Background())

	//确保参数正确
	if maxCapacity <= 0 {
		maxCapacity = defaultMaxCapacity
	}
	if maxWorkers <= 0 {
		maxWorkers = defaultMaxWorkers
	}

	pool := &ThreadPool{
		RequestTasks:    make(chan func(), maxCapacity),
		openerChan:      make(chan struct{}, maxWorkers),
		MaxWorkers:      maxWorkers,
		MaxIdleTime:     defaultIdleTimeout,
		MaxLifeTime:     0, //当最大存活时间未设置时，默认不回收
		ctx:             ctx,
		cancel:          cancel,
		maxCapacity:     maxCapacity,
		finishedTaskNum: 0,
		WaitTaskNum:     0,
		mutex:           sync.Mutex{},
		waitGroup:       sync.WaitGroup{},
		cleanerChan:     nil,
	}
	// Apply all options
	for _, opt := range options {
		opt(pool)
	}
	//初始化Worker线程
	pool.initWorks()

	go pool.WorkerOpener()

	return pool
}

// 当设置了minWorkers时，需要初始化minWorkers个Worker
func (pool *ThreadPool) initWorks() {
	select {
	case <-pool.ctx.Done():
		return
	default:
	}
	if pool.minWorkers > 0 {
		for i := int64(0); i < pool.minWorkers; i++ {
			worker := NewWorker(pool.ctx, pool)
			pool.IdleWorkers = append(pool.IdleWorkers, worker)
			atomic.AddInt64(&pool.IdleWorkersNum, 1)
		}
		atomic.AddInt64(&pool.OpenWorkers, pool.minWorkers)
	}
}

// 创建新Worker线程
func (pool *ThreadPool) WorkerOpener() {
	pool.waitGroup.Add(1)
	defer pool.waitGroup.Done()
	for {
		select {
		case <-pool.ctx.Done():
			return
		case <-pool.openerChan:
			pool.openNewWorker(pool.ctx)
		}
	}
}

// 创建新Worker线程
func (pool *ThreadPool) openNewWorker(ctx context.Context) {
	//判断线程池是否关闭
	if pool.stopped.Load() {
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
	}
	//判断是否需要创建新的Work线程
	if pool.getOpenWorkers() >= pool.MaxWorkers {
		return
	}
	//TODO 创建一个新的Work
	worker := NewWorker(ctx, pool)
	if pool.putWorkerThread(ctx, worker) {
		atomic.AddInt64(&pool.OpenWorkers, 1)
	}
}

// 将线程放入空闲队列
func (pool *ThreadPool) putWorkerThread(ctx context.Context, worker *Worker) bool {

	//判断线程池是否关闭
	if pool.stopped.Load() {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	default:
	}
	//存在请求任务
	if pool.getWaitTaskNum() > 0 {
		//随机获取一个任务请求
		select {
		case <-ctx.Done():
			return false
		default:
			go worker.Run()
			return true
		}
	} else if pool.MaxWorkers > 0 && pool.MaxWorkers > pool.getIdleWorkersNum() {

		worker.returnAt = &nowFunc
		pool.mutex.Lock()
		pool.IdleWorkers = append(pool.IdleWorkers, worker)
		atomic.AddInt64(&pool.IdleWorkersNum, 1)
		//当空闲协程数量大于1时，开启一个Clean线程
		if pool.getIdleWorkersNum() > 1 && pool.cleanerChan == nil {
			pool.cleanerChan = &struct{}{}
			go pool.CleanWorkers()
		}
		pool.mutex.Unlock()
		return true
	}
	return false
}

func (pool *ThreadPool) mayOpenWorkerThread() bool {
	if pool.stopped.Load() {
		return true
	}
	select {
	case <-pool.ctx.Done():
		return true
	default:
	}
	canOpenWorkers := pool.MaxWorkers - pool.getOpenWorkers()
	for canOpenWorkers > 0 {
		canOpenWorkers--
		pool.openerChan <- struct{}{}
		//TODO 这里应该让开启的Worker数量++
	}
	return true

}

// 关闭线程池
func (pool *ThreadPool) Stop() {
	if pool.stopped.Load() {
		return
	}
	// go pool.Purge()
	pool.cancel()
	pool.stopped.Store(true)
}

// 当线程池所有任务执行完成后关闭线程池
// 1.等待所有提交的Task执行 2.释放全部开启的Worker 3.释放所有开启的协程
func (pool *ThreadPool) gracefulStop() {
	if pool.stopped.Load() {
		return
	}
	timer := time.NewTicker(300 * time.Millisecond)
	for {
		select {
		case <-timer.C:
			if pool.getWaitTaskNum() == 0 {
				pool.Stop()
				go pool.Purge()
				pool.waitGroup.Wait()
				for range pool.RequestTasks {
				}
				return
			}
		}
	}
}

// 关闭所有的Goroutine
func (pool *ThreadPool) Purge() {
	ticker := time.NewTicker(200 * time.Millisecond)
	//关闭创建Worker的channel
	close(pool.openerChan)
	pool.waitGroup.Add(1)
	defer pool.waitGroup.Done()
	for {
		select {
		case <-ticker.C:
			if pool.getOpenWorkers() > 0 {
				//关闭开启的协程
				pool.RequestTasks <- nil
			} else {
				close(pool.RequestTasks)
				return
			}
		}
	}

}

// 阻塞添加Task
func (pool *ThreadPool) Submit(task func()) bool {
	if pool.stopped.Load() {
		return false
	}
	if task == nil {
		panic("Task is nil")
	}
	select {
	case <-pool.ctx.Done():
		return false
	default:
	}

	//1.判断是否有空闲的Work线程
	if pool.getIdleWorkersNum() > 0 {
		//TODO 可以开启一个Clean线程,清理空闲的Work
		pool.mutex.Lock()
		worker := pool.IdleWorkers[0]
		pool.IdleWorkers = pool.IdleWorkers[1:]
		atomic.AddInt64(&pool.IdleWorkersNum, -1)
		pool.mutex.Unlock()
		go worker.Run()
	}
	//2.判断是否需要创建新的Work线程，可以优化一下
	if pool.getOpenWorkers() < pool.MaxWorkers {
		pool.mayOpenWorkerThread()
	}
	// pool.mutex.Unlock()
	//阻塞添加任务到任务队列
	pool.RequestTasks <- task
	atomic.AddInt64(&pool.WaitTaskNum, 1)
	return true

}
func (pool *ThreadPool) TrySubmit(task func(), timeout time.Duration) {
	if pool.stopped.Load() {
		return
	}
	if task == nil {
		panic("Task is nil")
	}
	timer := time.NewTimer(timeout)
	pool.Submit(func() {
		select {
		case <-timer.C:
			// Deadline was reached, abort the task
		default:
			// Deadline not reached, execute the task
			defer timer.Stop()

			task()
		}
	})

}

func (pool *ThreadPool) CleanWorkers() {
	timer := time.NewTicker(time.Second * 60 * 3)

	pool.waitGroup.Add(1)

	defer func() {
		pool.waitGroup.Done()
		pool.mutex.Lock()
		pool.cleanerChan = nil
		pool.mutex.Unlock()
	}()
	for {
		select {
		case <-pool.ctx.Done():
			return
		case <-timer.C:
			return
		default:
		}
		pool.mutex.Lock()
		if pool.getIdleWorkersNum() > 0 {
			for i := 0; i < int(pool.getIdleWorkersNum()); i++ {
				worker := pool.IdleWorkers[i]
				//如果worker没有任务且超过最大空闲时间,则关闭该worker
				if worker.Expire(pool.MaxLifeTime, pool.MaxIdleTime) {
					pool.IdleWorkers = append(pool.IdleWorkers[:i], pool.IdleWorkers[i+1:]...)
					atomic.AddInt64(&pool.OpenWorkers, -1)
					atomic.AddInt64(&pool.IdleWorkersNum, -1)
					pool.mayOpenWorkerThread()
				}
			}
		}
		pool.mutex.Unlock()
	}
}
