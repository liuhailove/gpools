package gpools

import (
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"
)

func BlockConsumer(q *ArrayBlockingQueue, wg *sync.WaitGroup) {
	for i := 0; i < 100; i++ {
		val, closed := q.Take()
		if closed {
			break
		}
		time.Sleep(time.Second * 1)
		fmt.Printf("q take succeed, val: %v, serial num: %d\n", val, i)
	}
	wg.Done()
}

func BlockProducer(q *ArrayBlockingQueue, wg *sync.WaitGroup) {
	for i := 0; i < 100; i++ {
		if closed := q.Put("put_" + strconv.Itoa(i)); closed {
			break
		}
		fmt.Println("q put succeed, serial num: ", i)
	}
	fmt.Println("BlockProducer finished")
	q.DestroyFromSender()
	wg.Done()
}

func TestName(t *testing.T) {
	q, _ := NewArrayBlockingQueue(2)
	wg := new(sync.WaitGroup)
	wg.Add(4)
	// 一个生产者
	go BlockProducer(q, wg)
	/// 三个消费者
	go BlockConsumer(q, wg)
	go BlockConsumer(q, wg)
	go BlockConsumer(q, wg)
	wg.Wait()
}
