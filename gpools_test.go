package gpools

import (
	"fmt"
	"log"
	"testing"
	"time"
)

func TestGPools(t *testing.T) {
	var pools, err = NewPool(1, WithMaxBlockingTasks(1), WithPanicHandler(func(err interface{}) {
		log.Print(">>>>>>>>>>>callback too fast, match threadpool rejected handler(run now). error msg,", err)

	}))
	if err != nil {
		fmt.Println(err)
	}

	for i := 0; i < 2; i++ {
		pools.Submit(funcName(pools))
	}
	time.Sleep(10 * time.Second)
	fmt.Println("---------------------")
	for i := 0; i < 2; i++ {
		pools.Submit(funcName(pools))
	}

}

func funcName(pools *Pool) func() {
	return func() {
		fmt.Println("hello world")
		fmt.Printf("running:%d\n", pools.Running())
		fmt.Printf("free,%d\n", pools.Free())
	}
}
