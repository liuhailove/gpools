package gpools

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	var pools, err = NewPool(1, WithMaxQueueTasks(200), WithPanicHandler(func(err interface{}) {
		log.Print(">>>>>>>>>>>callback too fast, match threadpool rejected handler(run now). error msg,", err)

	}))
	if err != nil {
		fmt.Println(err)
	}

	go func() {
		for i := 0; i < 100; i++ {
			err = pools.Submit(funcName(pools, i))
			if err != nil {
				fmt.Println(err)
			}
			//go func(ii int) {
			//	err = pools.Submit(funcName(pools, ii))
			//	if err != nil {
			//		fmt.Println(err)
			//	}
			//}(i)
		}
	}()
	go func() {
		for i := 100; i < 200; i++ {
			err = pools.Submit(funcName(pools, i))
			if err != nil {
				fmt.Println(err)
			}
			//go func(ii int) {
			//	err = pools.Submit(funcName(pools, ii))
			//	if err != nil {
			//		fmt.Println(err)
			//	}
			//}(i)
		}
	}()
	time.Sleep(20 * time.Second)
}

func funcName(pools *Pool, i int) func() {
	return func() {
		fmt.Println("hello world,idx=" + strconv.Itoa(i) + "，time=" + time.Now().Format("2006-01-02 15:04:05"))
		//time.Sleep(time.Second)
		//fmt.Printf("running:%d\n", pools.Running())
		fmt.Printf("free,%d\n", pools.Free())
	}
}
