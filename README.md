# gpools
主要提供go-lang协程池实现
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

