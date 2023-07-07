package gpools

import (
	"fmt"
	"log"
	"strconv"
	"testing"
	"time"

	"github.com/shettyh/threadpool"
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

type MyTask struct {
	ID int
}

func (t *MyTask) Run() {
	fmt.Println("hello world id=" + strconv.Itoa(t.ID) + "，time=" + time.Now().Format("2006-01-02 15:04:05"))
	time.Sleep(time.Second)
}
func TestPool(t *testing.T) {
	pool := threadpool.NewThreadPool(10, 1000)
	for i := 0; i < 1000; i++ {
		go func() {
			task := &MyTask{ID: i}
			err := pool.Execute(task)
			if err != nil {
				fmt.Println(err)
			}
		}()

	}

	time.Sleep(time.Second * 1000)

}
