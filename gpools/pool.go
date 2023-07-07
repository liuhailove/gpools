package gpools

import (
	"fmt"
	"github.com/liuhailove/gpools"
	"github.com/liuhailove/gpools/internal"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrQueueFull = fmt.Errorf("queue is full, not able add the task")
)

// Pool 接收客户端的请求，通过循环利用协程，以便限制协程数量为设置的协程数量
type Pool struct {
	// capacity 协程池容量，如果为负数，则表示没有限制
	capacity int32

	// running 当前运行的协程池数量
	running int32

	// lock 用于工作队列保护
	lock sync.Locker

	// workers 存储可用的 workers 分片
	workers workerArray

	// state 用于通知关闭池自身
	state int32

	// cond 用于等待获取一个闲置的worker
	cond *sync.Cond

	// workerCache 在函数:retrieveWorker中加快获取可用的worker
	workerCache sync.Pool

	// queueSize 队列大小，当分配的协程数耗尽时会进入到阻塞队列中
	queueSize int

	options *Options

	jobQueue    chan func() // 阻塞的任务
	closeHandle chan bool   // Channel 用于关闭全部workers
}

// purgePeriodically 在一个独立的协程中清理过期的workers,是一个清道夫协程
func (p *Pool) purgePeriodically() {
	heartbeat := time.NewTicker(p.options.ExpiryDuration)
	defer heartbeat.Stop()

	for range heartbeat.C {
		if p.IsClosed() {
			break
		}

		p.lock.Lock()
		expiredWorkers := p.workers.retrieveExpiry(p.options.ExpiryDuration)
		p.lock.Unlock()

		// 通知过时的worker停止。
		// 这个通知必须在 p.lock 之外，因为 w.task
		// 如果有很多workers，可能会阻塞并且可能会消耗大量时间
		// 位于非本地 CPU 上。
		for i := range expiredWorkers {
			expiredWorkers[i].task <- nil
			expiredWorkers[i] = nil
		}

		// 可能会出现所有worker都已经清理完毕的情况（没有任何worker在运行）
		// 然而一些调用者仍然卡在“p.cond.Wait()”中，
		// 那么它应该唤醒所有这些调用者。
		if p.Running() == 0 {
			p.cond.Broadcast()
		}
	}
}

// NewPool 生成一个 gpools 的实例
func NewPool(size int, options ...Option) (*Pool, error) {
	opts := loadOptions(options...)

	if size <= 0 {
		size = -1
	}

	if expiry := opts.ExpiryDuration; expiry < 0 {
		return nil, gpools.ErrInvalidPoolExpiry
	} else if expiry == 0 {
		opts.ExpiryDuration = gpools.DefaultCleanIntervalTime
	}

	if opts.Logger == nil {
		opts.Logger = defaultLogger
	}

	p := &Pool{
		capacity: int32(size),
		lock:     internal.NewSpinLock(),
		options:  opts,
	}

	p.workerCache.New = func() interface{} {
		return &goWorker{
			pool: p,
			task: make(chan func(), workerChanCap),
		}
	}

	if p.options.PreAlloc {
		if size == -1 {
			return nil, gpools.ErrInvalidPreAllocSize
		}
		p.workers = newWorkerArray(loopQueueType, size)
	} else {
		p.workers = newWorkerArray(stackType, 0)
	}

	if opts.MaxQueueTasks > 0 {
		p.jobQueue = make(chan func(), opts.MaxQueueTasks)
		p.queueSize = opts.MaxQueueTasks
	}
	p.cond = sync.NewCond(p.lock)

	// 开启一个协程，以便定时清理过期的任务
	go p.purgePeriodically()

	// 协程分发
	go p.dispatch()

	return p, nil
}

var i = 0

// dispatch 监听jobQueue并使用workers处理任务
func (p *Pool) dispatch() {
	for {
		select {
		case job := <-p.jobQueue:
			var w *goWorker
			for {
				// 找到一个worker
				if w = p.retrieveWorker(); w != nil {
					// Got job
					break
				} else {
					//让出时间片，先让别的协议执行，它执行完，再回来执行此协程
					runtime.Gosched()
				}
			}
			func(task func()) {
				//i++
				//fmt.Println("in dispatch time" + time.Now().Format("2006-01-02 15:04:05") + ",idx=" + strconv.Itoa(i))

				// 执行任务
				w.task <- task
				//fmt.Println("dispatch time" + time.Now().Format("2006-01-02 15:04:05"+",idx="+strconv.Itoa(i)))

			}(job)

		case <-p.closeHandle:
			// Close thread threadpool
			return
		}
	}
}

// ---------------------------------------------------------------------------

// Submit 向池中提交一个task
func (p *Pool) Submit(task func()) error {
	if p.IsClosed() {
		return ErrPoolClosed
	}

	// 如果运行线程数小于pool的池数量，则直接申请一个协程运行任务
	if p.running < p.capacity {
		var w *goWorker
		if w = p.retrieveWorker(); w == nil {
			if len(p.jobQueue) == p.queueSize {
				return ErrPoolOverload
			}
			p.jobQueue <- task
		}
		w.task <- task
		return nil
	}

	// 如果阻塞队列可以继续放入任务，则放到阻塞队列中，否则报错
	if len(p.jobQueue) == p.queueSize {
		return ErrPoolOverload
	}
	p.jobQueue <- task
	return nil
}

// Running 当前运行的协程数量
func (p *Pool) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

// Free 返回工作队列中的可用的协程数，-1代表没有限制
func (p *Pool) Free() int {
	c := p.Cap()
	if c < 0 {
		return -1
	}
	return c - p.Running()
}

// Cap 返回池的容量
func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

// Tune 改变池的容量，注意对于无限的pool或者pre-allocation pool是无效的
func (p *Pool) Tune(size int) {
	if capacity := p.Cap(); capacity == -1 || size <= 0 || size == capacity || p.options.PreAlloc {
		return
	}
	atomic.StoreInt32(&p.capacity, int32(size))
}

func (p *Pool) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == CLOSED
}

// Release 关闭这个缓冲池，同时释放工作队列
func (p *Pool) Release() {
	atomic.StoreInt32(&p.state, CLOSED)
	p.lock.Lock()
	p.workers.reset()
	p.lock.Unlock()
	// 此时有可能一些调用方在等待retrieveWorkers()，所以我们需要唤醒他们，避免他们无穷的等待
	p.cond.Broadcast()
}

// Reboot 重启一个关闭的池
func (p *Pool) Reboot() {
	if atomic.CompareAndSwapInt32(&p.state, CLOSED, OPENED) {
		go p.purgePeriodically()
	}
}

// ---------------------------------------------------------------------------

// incRunning 增加当前运行的协程数
func (p *Pool) incRunning() {
	atomic.AddInt32(&p.running, 1)
}

// decRunning 减少当前运行的协程数量
func (p *Pool) decRunning() {
	atomic.AddInt32(&p.running, -1)
}

// retrieveWorker 返回一个可用的worker，以便运行tasks
func (p *Pool) retrieveWorker() (w *goWorker) {
	spawnWorker := func() {
		w = p.workerCache.Get().(*goWorker)
		w.run()
	}
	p.lock.Lock()

	w = p.workers.detach()
	if w != nil {
		// 第一次从queue中尝试抓去一个worker
		p.lock.Unlock()
	} else if capacity := p.Cap(); capacity == -1 || capacity > p.Running() {
		// 如果工作队列为空并且我们没有用完池容量，
		// 然后生成一个新的 worker goroutine
		p.lock.Unlock()
		spawnWorker()
	} else {
		p.lock.Unlock()
	}
	return
}

// revertWorker 把一个worker放回到可用池中，循环使用这个协程数
func (p *Pool) revertWorker(worker *goWorker) bool {
	if capacity := p.Cap(); (capacity > 0 && p.Running() > capacity) || p.IsClosed() {
		return false
	}
	worker.recycleTime = time.Now()
	p.lock.Lock()

	// 未来避免内存泄漏，增加一个double check
	if p.IsClosed() {
		p.lock.Unlock()
		return false
	}

	err := p.workers.insert(worker)
	if err != nil {
		p.lock.Unlock()
		return false
	}

	// 通知卡在“retrieveWorker()”中的调用者工作队列中有一个可用的worker
	p.cond.Signal()
	p.lock.Unlock()
	return true
}
