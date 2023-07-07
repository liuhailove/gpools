package gpools2

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	var pools, err = NewPool(1, WithMaximumQueueTasks(10), WithMaximumPoolSize(10), WithPanicHandler(func(err interface{}) {
		log.Print(">>>>>>>>>>>callback too fast, match threadpool rejected handler(run now). error msg,", err)

	}))
	if err != nil {
		fmt.Println(err)
	}

	for i := 0; i < 20; i++ {
		err = pools.Submit(funcName(pools, i))
		if err != nil {
			fmt.Println(err)
		}
	}

	//go func() {
	//	for i := 100; i < 200; i++ {
	//		err = pools.Submit(funcName(pools, i))
	//		if err != nil {
	//			fmt.Println(err)
	//		}
	//		//go func(ii int) {
	//		//	err = pools.Submit(funcName(pools, ii))
	//		//	if err != nil {
	//		//		fmt.Println(err)
	//		//	}
	//		//}(i)
	//	}
	//}()
	go func() {
		heartbeat := time.NewTicker(time.Second)
		defer heartbeat.Stop()

		for range heartbeat.C {
			var s = fmt.Sprintf("running pools:%d", pools.Running())
			fmt.Println(s)
			fmt.Println(pools.Cap())
			fmt.Println(pools.CorePoolSize())
		}
	}()
	time.Sleep(30 * time.Second)
}

func funcName(pools *Pool, i int) func() {
	return func() {
		fmt.Println("hello world,idx=" + strconv.Itoa(i) + "，time=" + time.Now().Format("2006-01-02 15:04:05"))
		//time.Sleep(time.Second)
		//fmt.Printf("running:%d\n", pools.Running())
		fmt.Printf("free,%d\n", pools.Free())
	}
}

func TestA(t *testing.T) {
	var lock = new(sync.RWMutex)

	lock.Lock()
	funcName2(lock)
	lock.Unlock()
}

func funcName2(lock *sync.RWMutex) {
	lock.Lock()
	fmt.Println("hello")
	lock.Unlock()
}
