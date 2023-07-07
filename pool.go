package gpools

import (
	"github.com/liuhailove/gpools/internal"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const maxBackoff = 64

// Pool 接收客户端的请求，通过循环利用协程，以便限制协程数量为设置的协程数量
type Pool struct {
	// corePoolSize 核心协程数，即便空闲也不会被回收
	corePoolSize int32

	// maximumPoolSize 池中的最大协程数，当空闲时会回收
	maximumPoolSize int32

	// maximumQueueSize 最大阻塞队列数，在没有核心线程可以申请时，task会优先进入到阻塞队列
	maximumQueueSize int32

	// 阻塞的任务
	jobQueue chan func()

	// closeDispatch 关闭分发信号
	closeDispatch chan bool

	// running 当前运行的协程池数量
	running int32

	// lock 用于工作队列保护
	lock sync.Locker

	// workers 存储可用的 workers 分片，maximumPoolSize
	workers workerArray

	// state 用于通知关闭池自身
	state int32

	// cond 用于等待获取一个闲置的worker
	cond *sync.Cond

	// workerCache 在函数:retrieveWorker中加快获取可用的worker
	workerCache sync.Pool

	options *Options
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
func NewPool(corePoolSize int, options ...Option) (*Pool, error) {
	opts := loadOptions(options...)

	if corePoolSize <= 0 {
		return nil, ErrInvalidPoolSize
	}

	if expiry := opts.ExpiryDuration; expiry < 0 {
		return nil, ErrInvalidPoolExpiry
	} else if expiry == 0 {
		opts.ExpiryDuration = DefaultCleanIntervalTime
	}

	// 最大协程数不能小于核心协程数
	if maximumPoolSize := opts.MaximumPoolSize; maximumPoolSize <= corePoolSize {
		opts.MaximumPoolSize = corePoolSize
	}

	// 最大缓冲队列不能小于0
	if maximumQueueTasks := opts.MaximumQueueTasks; maximumQueueTasks < 0 {
		return nil, ErrInvalidQueueSize
	}

	if opts.Logger == nil {
		opts.Logger = defaultLogger
	}

	p := &Pool{
		corePoolSize:     int32(corePoolSize),
		maximumPoolSize:  int32(opts.MaximumPoolSize),
		maximumQueueSize: int32(opts.MaximumQueueTasks),
		closeDispatch:    make(chan bool, 1),
		lock:             internal.NewSpinLock(),
		options:          opts,
	}

	if opts.MaximumQueueTasks > 0 {
		p.jobQueue = make(chan func(), opts.MaximumQueueTasks)
	}

	p.workerCache.New = func() interface{} {
		return &goWorker{
			pool: p,
			task: make(chan func(), workerChanCap),
		}
	}
	if p.options.PreAlloc {
		if corePoolSize == -1 {
			return nil, ErrInvalidPreAllocSize
		}
		p.workers = newWorkerArray(loopQueueType, corePoolSize)
	} else {
		p.workers = newWorkerArray(stackType, 0)
	}

	p.cond = sync.NewCond(p.lock)

	// 开启一个协程，以便定时清理过期的任务
	go p.purgePeriodically()

	// 队列任务分发
	go p.dispatch()

	return p, nil
}

// dispatch 监听jobQueue并使用workers处理任务
func (p *Pool) dispatch() {
	for {
		select {
		case job := <-p.jobQueue:
			var w *goWorker
			w = p.utilRetrieveWorker(true)
			// 执行任务
			w.task <- job
		case <-p.closeDispatch:
			return
		}
	}
}

func (p *Pool) utilRetrieveWorker(isCore bool) *goWorker {
	var w *goWorker
	backoff := 1
	for {
		// 找到一个worker
		if w = p.retrieveWorker(isCore); w != nil {
			// Got job
			return w
		}
		for i := 0; i < backoff; i++ {
			runtime.Gosched()
		}
		if backoff < maxBackoff {
			backoff <<= 1
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
	if p.Running() < int(p.corePoolSize) {
		var w = p.utilRetrieveWorker(true)
		if w != nil {
			w.task <- task
			return nil
		}
	}
	// 如果阻塞队列可以继续放入任务，则放到阻塞队列中，否则报错
	if len(p.jobQueue) < int(p.maximumQueueSize) {
		p.jobQueue <- task
		return nil
	}
	// 如果运行线程数小于pool的最大协程数量，则直接申请一个协程运行任务
	if p.Running() < int(p.maximumPoolSize) {
		var w = p.utilRetrieveWorker(false)
		if w != nil {
			w.task <- task
			return nil
		}
	}
	// 如果运行的协程数已经等于Pool的最大协程数，则返回池已经满
	return ErrPoolOverload
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

// Cap 返回池的容量 maximumPoolSize
func (p *Pool) Cap() int {
	return int(atomic.LoadInt32(&p.maximumPoolSize))
}

// CorePoolSize 返回核心池容量
func (p *Pool) CorePoolSize() int {
	return int(atomic.LoadInt32(&p.corePoolSize))
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
	p.closeDispatch <- true
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
// isCore 是否仅仅申请核心线程，在阻塞队列没有满之前，应该优先使用核心线程，
// 只有阻塞队列满时，才申请最发线程
func (p *Pool) retrieveWorker(isCore bool) (w *goWorker) {
	spawnWorker := func(recycleTime time.Time) {
		w = p.workerCache.Get().(*goWorker)
		w.recycleTime = recycleTime
		w.run()
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	w = p.workers.detach()
	if w != nil {
		// 第一次从queue中尝试抓去一个worker
		return
	}
	if corePoolSize := p.CorePoolSize(); corePoolSize > p.Running() {
		// 如果工作队列没有用完，
		// 然后生成一个新的 worker goroutine
		// 核心线程生成一个最大时间，表示用不过期
		spawnWorker(MaxTime)
		return
	}
	// 如果仅仅申请核心线程，则申请失败，直接返回
	if isCore {
		return
	}
	if maximumPoolSize := p.Cap(); maximumPoolSize > p.Running() {
		// 如果工作队列没有用完，
		// 然后生成一个新的 worker goroutine
		spawnWorker(time.Unix(0, 0))
		return
	}
	return
}

// revertWorker 把一个worker放回到可用池中，循环使用这个协程数
func (p *Pool) revertWorker(worker *goWorker) bool {
	if capacity := p.Cap(); capacity > 0 && p.Running() > capacity || p.IsClosed() {
		return false
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	var oldRecycleTime = worker.recycleTime
	worker.recycleTime = time.Now()
	if worker.recycleTime.Before(oldRecycleTime) {
		worker.recycleTime = oldRecycleTime
	}

	// 未来避免内存泄漏，增加一个double check
	if p.IsClosed() {
		return false
	}

	err := p.workers.insert(worker)
	if err != nil {
		return false
	}

	// 通知卡在“retrieveWorker()”中的调用者工作队列中有一个可用的worker
	p.cond.Signal()
	return true
}
