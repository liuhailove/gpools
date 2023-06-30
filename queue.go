package gpools

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Queue 接口
type Queue interface {
	Add(e interface{}) error
	Offer(e interface{}) bool
	Remove() (e interface{}, err error)
	Poll() (e interface{}, getE error)
	Element() (e interface{}, err error)
	Peek() (e interface{}, getE bool)
}

// BlockingQueue 接口
type BlockingQueue interface {
	Queue
	Put(e interface{}) (closed bool)    // closed状态可以标记队列是否关闭，用于当队列不可用时，能从阻塞中返回
	Take() (e interface{}, closed bool) // closed状态可以标记队列是否关闭，用于当队列不可用时，能从阻塞中返回
	OfferWithTimeout(e interface{}, timeout time.Duration) (offered bool, closed bool)
	PoolWithTimeout(timeout time.Duration) (e interface{}, err error, closed bool)
}

// ArrayBlockingQueue 及其实现
type ArrayBlockingQueue struct {
	objects  []interface{} // 存储元素
	taskIdx  int
	putIdx   int
	count    int
	mutex    *sync.Mutex // 保护临界区资源objects
	notEmpty chan bool   // 通知队列不为空的channel
	notFull  chan bool   // 通知队列没满的channel
}

// NewArrayBlockingQueue 初始化
func NewArrayBlockingQueue(cap int32) (*ArrayBlockingQueue, error) {
	if cap <= 0 {
		return nil, fmt.Errorf("cap of a queue should be greater than 0, got: %d", cap)
	}
	q := new(ArrayBlockingQueue)
	q.objects = make([]interface{}, cap)
	q.notEmpty = make(chan bool, 1)
	q.notFull = make(chan bool, 1)
	q.mutex = new(sync.Mutex)
	return q, nil
}

// DestroyFromSender 解除阻塞，由发送方调用，调用Put函数的一方
func (q *ArrayBlockingQueue) DestroyFromSender() {
	close(q.notEmpty)
}

// DestroyFromReceiver 解除阻塞，由接收方调用，调用Take函数的一方
func (q *ArrayBlockingQueue) DestroyFromReceiver() {
	close(q.notFull)
}

// enqueue 入队，仅供ArrayBlockingQueue其他函数调用，在调用此函数前，已经加锁保证了临界区资源访问的安全性
func (q *ArrayBlockingQueue) enqueue(e interface{}) {
	q.objects[q.putIdx] = e
	q.putIdx += 1
	if q.putIdx == len(q.objects) {
		q.putIdx = 0
	}
	q.count++
	select {
	case q.notEmpty <- true: //此处一定是非阻塞式发送非空的信号
	default:
		return
	}
}

// dequeue 出队，仅供ArrayBlockingQueue其他函数调用
func (q *ArrayBlockingQueue) dequeue() (e interface{}) {
	e = q.objects[q.taskIdx]
	q.objects[q.taskIdx] = nil
	q.taskIdx += 1
	if q.taskIdx == len(q.objects) {
		q.taskIdx = 0
	}
	q.count -= 1
	select {
	case q.notFull <- true: //此处一定是非阻塞式发送非空的信号
	default:
		return e
	}
	return e
}

// Queue接口的相关功能都比较容易实现，限于篇幅不在此展示

// Put 阻塞式Put
func (q *ArrayBlockingQueue) Put(e interface{}) (closed bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for q.count == len(q.objects) {
		q.mutex.Unlock() // 阻塞之前一定要释放锁，否则就会死锁
		_, ok := <-q.notFull
		if !ok {
			// queue closed
			q.mutex.Lock() // 判断队列是否已经关闭，关闭，则需要重新上锁，以便defer q.mutex.Unlock()解锁时不会报错
			return true    // 队列已关闭
		}
		q.mutex.Lock() // 阻塞解除时，重新上锁，再次判断队列是否已满
	}
	q.enqueue(e)
	return false
}

//Take 阻塞式Take
func (q *ArrayBlockingQueue) Take() (e interface{}, closed bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for q.count == 0 {
		q.mutex.Unlock() // 阻塞之前先解锁
		if _, ok := <-q.notEmpty; !ok {
			// queue closed
			q.mutex.Lock() // 判断队列是否已经关闭，关闭，则需要重新上锁，以便defer q.mutex.Unlock()解锁时不会报错
			return e, true
		}
		q.mutex.Lock() // 阻塞解除重新上锁
	}
	return q.dequeue(), false
}

// OfferWithTimeout 对于带timeout的take
func (q *ArrayBlockingQueue) OfferWithTimeout(e interface{}, timeout time.Duration) (offered bool, closed bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for q.count == len(q.objects) {
		q.mutex.Unlock()
		select {
		case _, ok := <-q.notFull:
			if !ok {
				// queue closed
				q.mutex.Lock()
				return false, true
			}
		case <-time.After(timeout): // 在阻塞的基础上，增加了超时机制
			q.mutex.Lock()
			return false, false
		}
		q.mutex.Lock()
	}
	q.enqueue(e)
	return true, false
}

// PollWithTimeout 对于带timeout的put
func (q *ArrayBlockingQueue) PollWithTimeout(timeout time.Duration) (e interface{}, err error, closed bool) {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	for q.count == 0 {
		q.mutex.Unlock()
		select {
		case _, ok := <-q.notEmpty:
			if !ok {
				// queue closed
				q.mutex.Lock()
				return e, nil, true
			}
		case <-time.After(timeout): // 增加了超时机制
			q.mutex.Lock()
			return e, errors.New("queue is empty"), false
		}
		q.mutex.Lock()
	}
	return q.dequeue(), nil, false
}
