package gool

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultIdleTimeout = 10 * time.Second
	defaultmaxCapacity = 10
	defaultmaxWorkers  = 8
)

type ThreadPool struct {
	mutex        sync.Mutex    //锁
	RequestTasks chan func()   //任务队列
	oppenerChan  chan struct{} //Work线程创建信号
	cleanerChan  *struct{}     //Work线程清除信号
	IdleWorkers  []*Worker     //空闲Work列表

	MaxWorkers  int64 //最大工作线程数
	OpenWorkers int64 //当前工作线程数
	minWorkers  int64 //最小工作线程数

	TaskId      int64 //任务id,线程安全的自生成任务Id
	maxCapacity int64 //最大任务数

	MaxIdleTime time.Duration //最大空闲时间
	MaxLifeTime time.Duration //最大存活时间

	finishedTaskNum int64 //已完成的任务数
	WaitTaskNum     int64 //等待执行的任务数

	stopped bool //是否关闭

	ctx    context.Context
	cancel context.CancelFunc
}

type Option func(*ThreadPool)

func WithMinWorkers(min int64) Option {
	return func(pool *ThreadPool) {
		if min >= pool.MaxWorkers {
			pool.minWorkers = 0
			return
		}
		pool.minWorkers = min
	}
}
func WithMaxLifeTime(maxLifeTime time.Duration) Option {
	return func(pool *ThreadPool) {
		pool.MaxLifeTime = maxLifeTime
	}
}
func WithIdleTimeout(idleTimeout time.Duration) Option {
	return func(pool *ThreadPool) {
		pool.MaxIdleTime = idleTimeout
	}
}

func New(maxCapacity, maxWorkers int64, options ...Option) *ThreadPool {

	// Instantiate the pool
	ctx, cancel := context.WithCancel(context.Background())

	//确保参数正确
	if maxCapacity <= 0 {
		maxCapacity = defaultmaxCapacity
	}
	if maxWorkers <= 0 {
		maxWorkers = defaultmaxWorkers
	}

	pool := &ThreadPool{
		RequestTasks:    make(chan func(), maxCapacity),
		oppenerChan:     make(chan struct{}, maxWorkers),
		MaxWorkers:      maxWorkers,
		MaxIdleTime:     defaultIdleTimeout,
		MaxLifeTime:     0, //当最大存活时间未设置时，默认不回收
		ctx:             ctx,
		cancel:          cancel,
		maxCapacity:     maxCapacity,
		finishedTaskNum: 0,
		WaitTaskNum:     0,
		mutex:           sync.Mutex{},
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
		}
		atomic.AddInt64(&pool.OpenWorkers, pool.minWorkers)
	}
}

// 创建新Worker线程
func (pool *ThreadPool) WorkerOpener() {
	for {
		select {
		case <-pool.ctx.Done():
			return
		case <-pool.oppenerChan:
			pool.openNewWorker(pool.ctx)
		}
	}
}

// 创建新Worker线程
func (pool *ThreadPool) openNewWorker(ctx context.Context) {
	//判断线程池是否关闭
	if pool.stopped {
		return
	}
	select {
	case <-ctx.Done():
		return
	default:
	}
	pool.mutex.Lock()
	defer pool.mutex.Unlock()

	//判断是否需要创建新的Work线程
	if pool.OpenWorkers >= pool.MaxWorkers {
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
	if pool.stopped {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	default:
	}
	//存在请求任务
	if pool.WaitTaskNum > 0 {
		//随机获取一个任务请求
		select {
		case <-ctx.Done():
			return false
		default:
			go worker.Run()
			return true
		}
	} else if pool.MaxWorkers > 0 && pool.MaxWorkers > pool.OpenWorkers {
		worker.returnAt = &nowFunc
		pool.IdleWorkers = append(pool.IdleWorkers, worker)
		//当空闲协程数量大于1时，开启一个Clean线程
		if len(pool.IdleWorkers) > 1 && pool.cleanerChan == nil {
			pool.cleanerChan = &struct{}{}
			go pool.CleanWorkers()
		}
		return true
	}
	return false
}

func (pool *ThreadPool) mayOpenWorkerThread() bool {
	if pool.stopped {
		return true
	}
	select {
	case <-pool.ctx.Done():
		return true
	default:
	}
	canOpenWorkers := pool.MaxWorkers - pool.OpenWorkers
	for canOpenWorkers > 0 {
		canOpenWorkers--
		pool.oppenerChan <- struct{}{}
	}
	return true

}

// 关闭线程池
func (pool *ThreadPool) Stop() {
	if pool.stopped {
		return
	}
	pool.cancel()
	pool.mutex.Lock()
	defer pool.mutex.Unlock()
	pool.stopped = true
}

// 阻塞添加Task
func (pool *ThreadPool) Submit(task func()) bool {
	if pool.stopped {
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
	pool.mutex.Lock()

	//1.判断是否有空闲的Work线程
	idleWorkersNum := len(pool.IdleWorkers)
	if idleWorkersNum > 0 {
		//TODO 可以开启一个Clean线程,清理空闲的Work
		worker := pool.IdleWorkers[0]
		pool.IdleWorkers = pool.IdleWorkers[1:]
		// 开启一个协程执行任务
		go worker.Run()

		pool.mutex.Unlock()
		return true
	}
	pool.mutex.Unlock()
	//2.判断是否需要创建新的Work线程，可以优化一下
	if pool.OpenWorkers < pool.MaxWorkers {
		pool.mayOpenWorkerThread()
	}
	// pool.mutex.Unlock()
	//阻塞添加任务到任务队列
	pool.RequestTasks <- task
	atomic.AddInt64(&pool.WaitTaskNum, 1)
	return true

}
func (pool *ThreadPool) Trysubmit(task func(), timeout time.Duration) {
	if pool.stopped {
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
	for {
		select {
		case <-pool.ctx.Done():
			return
		case <-timer.C:
			pool.cleanerChan = nil
			return
		default:
		}
		pool.mutex.Lock()
		idleWorkerNum := len(pool.IdleWorkers)
		if idleWorkerNum > 0 {
			for i := 0; i < idleWorkerNum; i++ {
				worker := pool.IdleWorkers[i]
				//如果worker没有任务且超过最大空闲时间,则关闭该worker
				if worker.Expire(pool.MaxLifeTime, pool.MaxIdleTime) {
					worker.Close()
					pool.IdleWorkers = append(pool.IdleWorkers[:i], pool.IdleWorkers[i+1:]...)
					atomic.AddInt64(&pool.OpenWorkers, -1)
					pool.mayOpenWorkerThread()
				}
			}
		}
		pool.mutex.Unlock()
	}
}
