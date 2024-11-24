package gool

import (
	"fmt"
	"testing"
	"time"
)

func TestMain(test *testing.T) {
	threadPool := New(5, 5)
	threadPool.Submit(func() {

		println("Hello World")

	})
	// threadPool.Submit(func() {

	// 	println("Hello World")

	// })
	for i := 0; i < 10; i++ {
		threadPool.Submit(func() {
			fmt.Println("Hello World", i)
		})
	}

	time.Sleep(10 * time.Second)
	threadPool.Stop()

}
