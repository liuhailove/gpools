package gpools

import (
	"runtime"
	"time"
)

// goWorker 是运行任务的实际执行者，
// 它启动一个 goroutine 接受任务和
// 执行函数调用
type goWorker struct {
	// pool 此worker的拥有者
	pool *Pool

	// task 一个需要完成的job
	task chan func()

	// recycleTime 将worker放回队列时将更新
	recycleTime time.Time
}

// run 启动一个 goroutine 来重复执行函数调用
func (w *goWorker) run() {
	w.pool.incRunning()
	go func() {
		defer func() {
			w.pool.decRunning()
			w.pool.workerCache.Put(w)
			if p := recover(); p != nil {
				if ph := w.pool.options.PanicHandler; ph != nil {
					ph(p)
				} else {
					w.pool.options.Logger.Printf("worker exits from a panic: %v\n", p)
					var buf [4096]byte
					n := runtime.Stack(buf[:], false)
					w.pool.options.Logger.Printf("worker exits from panic: %s\n", string(buf[:n]))
				}
			}
			// Call Signal() here in case there are goroutines waiting for available workers.
			w.pool.cond.Signal()
		}()

		for f := range w.task {
			if f == nil {
				return
			}
			f()
			if ok := w.pool.revertWorker(w); !ok {
				return
			}
		}
	}()
}
