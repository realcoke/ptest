package main

import (
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/realcoke/ptest"
)

func main() {
	log.Println("init")
	// Collector collects raw data
	collector := ptest.NewCollector()
	// Monitor processes raw data to stat
	m := ptest.NewMonitor(collector.ResultChan)
	// WebView shows performance test result on web
	ptest.NewWebViewer(m.ResultChan, ":9090")

	log.Println("start")
	stop := false
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			for !stop {
				start := time.Now()
				time.Sleep(time.Millisecond * time.Duration(rand.Intn(99)))
				result := true
				if rand.Intn(10) < 2 {
					result = false
				}
				// Report test result
				collector.Report(start, result)
			}
			wg.Done()
		}()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	log.Println("stop")
	stop = true
	wg.Wait()
	log.Println("goroutines - stopped")

	// this will close chains of channels
	// 1. input and output channels of the collector
	// 2. input and output channels of the monitor
	// 3. input and output channels of the viewview
	collector.Stop()
	time.Sleep(time.Second)
}
