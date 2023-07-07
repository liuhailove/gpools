package gpools

import "time"

// Option optional操作方法
type Option func(opts *Options)

func loadOptions(options ...Option) *Options {
	opts := new(Options)
	for _, option := range options {
		option(opts)
	}
	return opts
}

// Options 新建一个pool时的全部选项
type Options struct {
	// ExpiryDuration 是 scavenger Goroutine 清理那些过期的 Worker 的时间，
	// scavenger 在每个“ExpiryDuration”期间扫描所有workers，
	// 并清理超过 `ExpiryDuration` 的时间worker。
	ExpiryDuration time.Duration

	// PreAlloc 初始化Pool时是否进行内存预分配
	PreAlloc bool

	// MaximumPoolSize 最大协程数
	MaximumPoolSize int

	// pools中可以缓存的最大任务数
	MaximumQueueTasks int

	// PanicHandler 用于处理来自每个工作协程的panics。
	// 如果为nil，panics将再次从工作协程中抛出。
	PanicHandler func(interface{})

	// Logger 用户可以定制化日志输出。如果没有设置，则会使用log包下的标准日志输出
	Logger Logger
}

// WithOptions 接收全部的options配置
func WithOptions(options Options) Option {
	return func(opts *Options) {
		*opts = options
	}
}

// WithExpiryDuration 设置清理协程的间隔时间
func WithExpiryDuration(expiryDuration time.Duration) Option {
	return func(opts *Options) {
		opts.ExpiryDuration = expiryDuration
	}
}

//
//// WithPreAlloc 是否提前分配workers
//func WithPreAlloc(preAlloc bool) Option {
//	return func(opts *Options) {
//		opts.PreAlloc = preAlloc
//	}
//}

// WithMaximumQueueTasks 当协程池没有空闲协程时，可以缓冲的最大协程数
func WithMaximumQueueTasks(maximumQueueTasks int) Option {
	return func(opts *Options) {
		opts.MaximumQueueTasks = maximumQueueTasks
	}
}

// WithMaximumPoolSize 最大协程数
func WithMaximumPoolSize(maximumPoolSize int) Option {
	return func(opts *Options) {
		opts.MaximumPoolSize = maximumPoolSize
	}
}

// WithPanicHandler panic处理方法
func WithPanicHandler(panicHandler func(interface{})) Option {
	return func(opts *Options) {
		opts.PanicHandler = panicHandler
	}
}

// WithLogger 设置定制的logger
func WithLogger(logger Logger) Option {
	return func(opts *Options) {
		opts.Logger = logger
	}
}
