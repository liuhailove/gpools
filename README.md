# gpools
一个高效的golang协程池组件
# 参数
- corePoolSize 核心协程数，一旦被分配不会释放
- maximumQueueTasks 最大阻塞队列，当并发度超过核心协程数时，多出的任务将会被放到阻塞队列中，直到有合适的协程处理
- maximumPoolSize 当阻塞队列满时，会继续分配协程处理任务，当空闲时会自动回收，当并发读超过此值时则报协程池满异常
- expiryDuration 空闲协程回收时间，只有maximumPoolSize-corePoolSize中的协程才会被回收，默认1s后回收
- panicHandler 协程异常处理，如果协程处理出现异常，协程会被回收，包含corePoolSize中的协程
- logger 日志
# 运行步骤
- 1.通过NewPool方法构造一个协程池，其中如下代码定义了生成goWorker的方法
```go
	p.workerCache.New = func() interface{} {
		return &goWorker{
			pool: p,
			task: make(chan func(), workerChanCap),
		}
	}
```
- 2.其中一个协程用于清理过期的goWorker，当task不在繁忙时，协程会被回收
- 3.通过Submit提交一个任务，submit中通过retrieveWorker获取一个可用的goWorker，然后把任务通过chan发送给goWorker
- 4.goWorker增加正在运行的任务数量，同时遍历task chan，回调业务方法，回调后会把goWorker放入到队列中，以便循环使用
- 5.goWorker会一直for range task chan，直到有任务或者回调发生异常或者task为空
- 6.如果回调方法发生异常，则会销毁go协程，并通知池可以重新创建一个协程，同时把运行协程数减一
- 7.若超过过期时间没有任务要运行，则会执行清理程序
- 8.清理程序会把过期的task设置为空，以便for range中断，进入协程回收

