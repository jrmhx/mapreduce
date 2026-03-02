package main

//
// start the coordinator process, which is implemented
// in ../../coordinator.go
//
// go run mrcoordinator.go pg*.txt
//
// Please do not change this file.
//

import (
	"fmt"
	"os"
	"time"

	mr "github.com/jrmhx/mapreduce"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: mrcoordinator sockname inputfiles...\n")
		os.Exit(1)
	}

	m := mr.MakeCoordinator(os.Args[1], os.Args[2:], 10)
	for m.Done() == false {
		time.Sleep(time.Second)
	}

	time.Sleep(time.Second)
}
