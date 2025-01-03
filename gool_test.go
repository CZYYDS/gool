package gool

import (
	"fmt"
	"testing"
	"time"
)

func TestMain(test *testing.T) {
	threadPool := New(10, 5)
	// threadPool.Submit(func() {

	// 	println("Hello World")

	// })
	for i := 0; i < 100; i++ {
		threadPool.Submit(func() {
			fmt.Println("Hello World", i)
		})
	}

	time.Sleep(10 * time.Second)
	fmt.Println(threadPool.finishedTaskNum) //7
	fmt.Println(threadPool.OpenWorkers)     //5
	fmt.Println(threadPool.WaitTaskNum)     //3
	fmt.Println(threadPool.IdleWorkersNum)  //5

	threadPool.Stop()

}

func TestStop(test *testing.T) {
	threadPool := New(10, 5)
	for i := 0; i < 10; i++ {
		threadPool.Submit(func() {
			time.Sleep(3 * time.Second)
			fmt.Println("Hello World", i)
		})
	}

	time.Sleep(1 * time.Second)
	threadPool.gracefulStop()
	fmt.Println(threadPool.finishedTaskNum) //7
	fmt.Println(threadPool.OpenWorkers)     //5
	fmt.Println(threadPool.WaitTaskNum)     //3
	fmt.Println(threadPool.IdleWorkersNum)  //5
	time.Sleep(25 * time.Second)
}
