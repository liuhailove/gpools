package gpools

import "time"

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
}
