package gpools

import (
	"fmt"
	"log"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	var pools, err = NewPool(5, WithMaximumQueueTasks(100), WithMaximumPoolSize(20), WithPanicHandler(func(err interface{}) {
		log.Print(">>>>>>>>>>>callback too fast, match threadpool rejected handler(run now). error msg,", err)

	}))
	if err != nil {
		fmt.Println(err)
	}

	for i := 0; i < 121; i++ {
		go func(ii int) {
			err = pools.Submit(funcName(pools, ii))
			if err != nil {
				fmt.Println(err)
			}
		}(i)
	}
	go func() {
		heartbeat := time.NewTicker(time.Second)
		defer heartbeat.Stop()

		for range heartbeat.C {
			var s = fmt.Sprintf("running pools:%d,time=%s", pools.Running(), time.Now().Format("2006-01-02 15:04:05"))
			fmt.Println(s)
			fmt.Println(pools.Cap())
			fmt.Println(pools.CorePoolSize())
		}
	}()
	time.Sleep(60 * time.Second)
}

func funcName(pools *Pool, i int) func() {
	return func() {
		fmt.Println("hello world,idx=" + strconv.Itoa(i) + "，time=" + time.Now().Format("2006-01-02 15:04:05"))
		time.Sleep(time.Second)
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

func TestSort(t *testing.T) {
	var arr []int
	arr2 := insert(1, arr)
	fmt.Println(arr2)
	arr2 = insert(5, arr2)
	fmt.Println(arr2)
	arr2 = insert(4, arr2)
	fmt.Println(arr2)
	arr2 = insert(2, arr2)
	fmt.Println(arr2)
	arr2 = insert(6, arr2)
	fmt.Println(arr2)
}

func insert(ins int, arr []int) []int {
	var findIdx = len(arr)
	for i := len(arr) - 1; i >= 0; i-- {
		if arr[i] > ins {
			findIdx = i
		}
	}
	var arr2 = make([]int, len(arr)+1)
	copy(arr2[:findIdx], arr[:findIdx])
	arr2[findIdx] = ins
	copy(arr2[findIdx+1:], arr[findIdx:])
	return arr2
}
