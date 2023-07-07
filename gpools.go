package gpools

import (
	"errors"
	"log"
	"math"
	"os"
	"runtime"
	"time"
)

const (
	// DefaultGPoolSize 协程池默认数量.
	DefaultGPoolSize = math.MaxInt32

	// DefaultCleanIntervalTime 协程默认清理时间.
	DefaultCleanIntervalTime = time.Second
)

const (
	// OPENED 代表pool是打开的
	OPENED = iota

	// CLOSED 代表池是关闭的
	CLOSED
)

var (
	// MaxTime 最大时间
	MaxTime = time.Unix(1<<63-62135596801, 999999999)
)

var (
	// Error types for the GPools API.
	//---------------------------------------------------------------------------

	// ErrInvalidPoolSize will be returned when setting a negative number as pool capacity, this error will be only used
	// by pool with func because pool without func can be infinite by setting up a negative capacity.
	ErrInvalidPoolSize = errors.New("invalid size for pool")

	ErrInvalidQueueSize = errors.New("invalid size for queue size")

	// ErrLackPoolFunc will be returned when invokers don't provide function for pool.
	ErrLackPoolFunc = errors.New("must provide function for pool")

	// ErrInvalidPoolExpiry will be returned when setting a negative number as the periodic duration to purge goroutines.
	ErrInvalidPoolExpiry = errors.New("invalid expiry for pool")

	// ErrPoolClosed will be returned when submitting task to a closed pool.
	ErrPoolClosed = errors.New("this pool has been closed")

	// ErrPoolOverload will be returned when the pool is full and no workers available.
	ErrPoolOverload = errors.New("too many goroutines blocked on submit")

	// ErrInvalidPreAllocSize will be returned when trying to set up a negative capacity under PreAlloc mode.
	ErrInvalidPreAllocSize = errors.New("can not set up a negative capacity under PreAlloc mode")

	//---------------------------------------------------------------------------
	workerChanCap = func() int {
		// Use blocking channel if GOMAXPROCS=1.
		// This switches context from sender to receiver immediately,
		// which results in higher performance (under go1.5 at least).
		if runtime.GOMAXPROCS(0) == 1 {
			return 0
		}

		// Use non-blocking workerChan if GOMAXPROCS>1,
		// since otherwise the sender might be dragged down if the receiver is CPU-bound.
		return 1
	}()

	defaultLogger = Logger(log.New(os.Stderr, "", log.LstdFlags))

	// Init a instance pool when importing GPools.
	defaultGPool, _ = NewPool(DefaultGPoolSize)
)

// Submit 向pool中提交一个task
func Submit(task func()) error {
	return defaultGPool.Submit(task)
}

// Running 返回当前运行的协程数
func Running() int {
	return defaultGPool.Running()
}

// Cap 返回默认池的容量
func Cap() int {
	return defaultGPool.Cap()
}

// Free 返回可用的协程数
func Free() int {
	return defaultGPool.Free()
}

// Release 关闭默认的pol
func Release() {
	defaultGPool.Release()
}

// Reboot 重启默认的pool.
func Reboot() {
	defaultGPool.Reboot()
}
