package gpools

import (
	"github.com/liuhailove/gpools/internal"
	"sync"
	"sync/atomic"
	"time"
)

// PoolWithFunc 接收client的任务，通过循环利用goroutines来限制goroutines的数量
type PoolWithFunc struct {
	// capacity 池的容量
	capacity int32

	// running 档案运行协程的数量
	running int32

	// lock 保护当前工作队列的锁
	lock sync.Locker

	// workers 存储当前可用workers的一个切片
	workers []*goWorkerWithFunc

	// state 用于通知池的关闭
	state int32

	// cond 用于等待一个闲置worker
	cond *sync.Cond

	// poolFunc 处理tasks的方法
	poolFunc func(interface{})

	// workerCache 在retrieveWorker中用户加快获取一个可用的worker
	workerCache sync.Pool

	// blockingNum 阻塞到pool.Submit的协程数，被pool.lock所保护
	blockingNum int

	options *Options
}

// purgePeriodically 作为一个垃圾收集器，在一个独立协程中定时运行清理过期的workers
func (p *PoolWithFunc) purgePeriodically() {
	heartbeat := time.NewTicker(p.options.ExpiryDuration)
	defer heartbeat.Stop()

	var expiredWorkers []*goWorkerWithFunc
	for range heartbeat.C {
		if p.IsClosed() {
			break
		}
		currentTime := time.Now()
		p.lock.Lock()
		idleWorkers := p.workers
		n := len(idleWorkers)
		var i int
		for i = 0; i < n && currentTime.Sub(idleWorkers[i].recycleTime) > p.options.ExpiryDuration; i++ {
		}
		expiredWorkers = append(expiredWorkers[:0], idleWorkers[:i]...)
		if i > 0 {
			m := copy(idleWorkers, idleWorkers[i:])
			for i = m; i < n; i++ {
				idleWorkers[i] = nil
			}
			p.workers = idleWorkers[:m]
		}
		p.lock.Unlock()

		// 通知过时的worker停止。
		// 这个通知必须在 p.lock 之外，因为 w.task
		// 如果有很多worker，可能会阻塞并且可能会消耗大量时间
		// 位于非本地 CPU 上。
		for i, w := range expiredWorkers {
			w.args <- nil
			expiredWorkers[i] = nil
		}

		// 有可能所有的workers已经被清理了（没有一个worker在运行中），
		// 然而还是有一个调用者阻塞在p.cond.Wait()，
		// 此时我们需要唤醒全部的调用者
		if p.Running() == 0 {
			p.cond.Broadcast()
		}
	}
}

// NewPoolWithFunc 按照具体的方法产生一个gpools
func NewPoolWithFunc(size int, pf func(interface{}), options ...Option) (*PoolWithFunc, error) {
	if size <= 0 {
		size = -1
	}

	if pf == nil {
		return nil, ErrLackPoolFunc
	}

	opts := loadOptions(options...)

	if expiry := opts.ExpiryDuration; expiry < 0 {
		return nil, ErrInvalidPoolExpiry
	} else if expiry == 0 {
		opts.ExpiryDuration = DefaultCleanIntervalTime
	}

	if opts.Logger == nil {
		opts.Logger = defaultLogger
	}

	p := &PoolWithFunc{
		capacity: int32(size),
		poolFunc: pf,
		lock:     internal.NewSpinLock(),
		options:  opts,
	}
	p.workerCache.New = func() interface{} {
		return &goWorkerWithFunc{
			pool: p,
			args: make(chan interface{}, workerChanCap),
		}
	}
	if p.options.PreAlloc {
		if size == -1 {
			return nil, ErrInvalidPreAllocSize
		}
		p.workers = make([]*goWorkerWithFunc, 0, size)
	}
	p.cond = sync.NewCond(p.lock)

	// 开启一个协程清理过去的workers
	go p.purgePeriodically()

	return p, nil
}

//---------------------------------------------------------------------------

// Invoke 向池中提交一个任务
func (p *PoolWithFunc) Invoke(args interface{}) error {
	if p.IsClosed() {
		return ErrPoolClosed
	}
	var w *goWorkerWithFunc
	if w = p.retrieveWorker(); w == nil {
		return ErrPoolOverload
	}
	w.args <- args
	return nil
}

// Running 返回当前运行的协程数
func (p *PoolWithFunc) Running() int {
	return int(atomic.LoadInt32(&p.running))
}

// Free 返回可以用于工作的协程数，-1表示这个池是没有限制的
func (p *PoolWithFunc) Free() int {
	c := p.Cap()
	if c < 0 {
		return -1
	}
	return c - p.Running()
}

// Cap 返回池的容量
func (p *PoolWithFunc) Cap() int {
	return int(atomic.LoadInt32(&p.capacity))
}

// Tune 修改池的容量，如果池是无限制的或者预分配的，则无效
func (p *PoolWithFunc) Tune(size int) {
	if capacity := p.Cap(); capacity == -1 || size <= 0 || size == capacity || p.options.PreAlloc {
		return
	}
	atomic.StoreInt32(&p.capacity, int32(size))
}

// IsClosed 指示当前pool是不是关闭了
func (p *PoolWithFunc) IsClosed() bool {
	return atomic.LoadInt32(&p.state) == CLOSED
}

// Release 关闭这个池，并释放工作队列
func (p *PoolWithFunc) Release() {
	atomic.StoreInt32(&p.state, CLOSED)
	p.lock.Lock()
	idleWorkers := p.workers
	for _, w := range idleWorkers {
		w.args <- nil
	}
	p.workers = nil
	p.lock.Unlock()
	// 此时有可能一些调用者在等待retrieveWorker()，所以我们需要唤醒他们，比买呢他们无穷的阻塞
	p.cond.Broadcast()
}

// Reboot 重启一个关闭的pools
func (p *PoolWithFunc) Reboot() {
	if atomic.CompareAndSwapInt32(&p.state, CLOSED, OPENED) {
		go p.purgePeriodically()
	}
}

//---------------------------------------------------------------------------

// incRunning 增加当前运行的协程数
func (p *PoolWithFunc) incRunning() {
	atomic.AddInt32(&p.running, 1)
}

// decRunning 减少当前运行的协程数
func (p *PoolWithFunc) decRunning() {
	atomic.AddInt32(&p.running, -1)
}

// retrieveWorker 返回一个可用的worker，以便可以运行tasks
func (p *PoolWithFunc) retrieveWorker() (w *goWorkerWithFunc) {
	spawnWorker := func() {
		w = p.workerCache.Get().(*goWorkerWithFunc)
		w.run()
	}

	p.lock.Lock()
	idleWorkers := p.workers
	n := len(idleWorkers) - 1
	if n >= 0 { // 首先从队列中找到一个可用的worker
		w = idleWorkers[n]
		idleWorkers[n] = nil
		p.workers = idleWorkers[:n]
		p.lock.Unlock()
	} else if capacity := p.Cap(); capacity == -1 || capacity > p.Running() {
		// 如果工作队列是空的，或者池的容量没有耗尽，
		// 那么我们直接产生一个新的工作协程
		p.lock.Unlock()
		spawnWorker()
	} else {
		// 否则，我们必须要阻塞等待，直到有一个worker放入到pool中
		if p.options.Nonblocking {
			p.lock.Unlock()
			return
		}
	retry:
		if p.options.MaxBlockingTasks != 0 && p.blockingNum >= p.options.MaxBlockingTasks {
			p.lock.Unlock()
			return
		}
		p.blockingNum++
		p.cond.Wait() // 阻塞等待一个可用的worker
		p.blockingNum--
		var nw int
		if nw = p.Running(); nw == 0 { // 唤醒垃圾收集器
			p.lock.Unlock()
			if !p.IsClosed() {
				spawnWorker()
			}
			return
		}
		l := len(p.workers) - 1
		if l < 0 {
			if nw < capacity {
				p.lock.Unlock()
				spawnWorker()
				return
			}
			goto retry
		}
		w = p.workers[l]
		p.workers[l] = nil
		p.workers = p.workers[:l]
		p.lock.Unlock()
	}
	return
}

// revertWorker 把worker放到空闲池中，以便循环利用go协程
func (p *PoolWithFunc) revertWorker(worker *goWorkerWithFunc) bool {
	if capacity := p.Cap(); (capacity > 0 && p.Running() > capacity) || p.IsClosed() {
		return false
	}
	worker.recycleTime = time.Now()
	p.lock.Lock()

	// To avoid memory leaks, add a double check in the lock scope.
	// Issue: https://github.com/panjf2000/ants/issues/113
	if p.IsClosed() {
		p.lock.Unlock()
		return false
	}

	p.workers = append(p.workers, worker)

	// 通知阻塞到'retrieveWorker()'的invoker，目前有一个空用的worker
	p.cond.Signal()
	p.lock.Unlock()
	return true

}
