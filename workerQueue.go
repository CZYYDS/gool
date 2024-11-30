package gool

import (
	"errors"
	"sync"
)

const (
	WaitForNextElementChanCapacity = 1000
)

var (
	ErrQueueFull  = errors.New("queue is full")
	ErrQueueEmpty = errors.New("queue is empty")
)

type WorkerQueue struct {
	slice    []*Worker
	rwmutex  sync.Mutex
	head     int
	tail     int
	count    int
	capacity int
}

func NewWorkerQueue(size int) *WorkerQueue {
	queue := &WorkerQueue{
		capacity: size,
		slice:    make([]*Worker, 0, size),
		rwmutex:  sync.Mutex{},
		head:     0,
		tail:     0,
		count:    0,
	}

	return queue
}

// 队尾入队操作
func (queue *WorkerQueue) Enqueue(value *Worker) error {
	queue.rwmutex.Lock()
	defer queue.rwmutex.Unlock()

	if len(queue.slice) == queue.capacity {
		return ErrQueueFull
	}
	queue.slice[queue.tail] = value
	queue.tail = (queue.tail + 1) % queue.capacity
	queue.count++
	return nil
}

// 队首出队操作
func (queue *WorkerQueue) Dequeue() (*Worker, error) {
	queue.rwmutex.Lock()
	defer queue.rwmutex.Unlock()

	if len(queue.slice) == 0 {
		return nil, ErrQueueEmpty
	}
	value := queue.slice[queue.head]
	queue.head = (queue.head + 1) % queue.capacity
	queue.count--

	return value, nil
}
func (st *WorkerQueue) GetFirst() *Worker {
	st.rwmutex.Lock()
	defer st.rwmutex.Unlock()
	if len(st.slice) == 0 {
		return nil
	}

	return st.slice[0]
}

func (st *WorkerQueue) RemoveFirst() error {
	st.rwmutex.Lock()
	defer st.rwmutex.Unlock()
	if len(st.slice) == 0 {
		return ErrQueueEmpty
	}
	st.slice = st.slice[1:]

	return nil
}
func (st *WorkerQueue) RemoveLast() error {
	st.rwmutex.Lock()
	defer st.rwmutex.Unlock()
	if len(st.slice) == 0 {
		return ErrQueueEmpty
	}
	st.slice = st.slice[:len(st.slice)-1]
	return nil
}

// GetLen returns the number of enqueued elements
func (st *WorkerQueue) GetLen() int {
	st.rwmutex.Lock()
	defer st.rwmutex.Unlock()

	return st.count
}

// GetCap returns the queue's capacity
func (st *WorkerQueue) GetCap() int {
	st.rwmutex.Lock()
	defer st.rwmutex.Unlock()

	return cap(st.slice)
}
