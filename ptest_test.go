package ptest_test

import (
	"sync"
	"testing"
	"time"

	"github.com/realcoke/ptest"
)

func TestCollector(t *testing.T) {
	t.Log("init")
	collector := ptest.NewCollector()

	t.Log("start")
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		// generate 100 goroutines
		go func() {
			// report 5000 test data per each goroutine
			// it will task about 5 sec, since it sleeps 1 millisecond before each report
			for j := 0; j < 5000; j++ {
				start := time.Now()
				time.Sleep(time.Millisecond)
				collector.Report(start, (j % 10 != 0))
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		collector.Stop()
		t.Log("stop")
	}()

	success := 0
	failure := 0

	for {
		s, more := <-collector.ResultChan
		if !more {
			break
		}
		success += len(s.Success)
		failure += len(s.Failure)
		t.Log(s.Time, len(s.Success))
	}
	t.Log(success)
	t.Log(failure)
	if success != 450000 || failure != 50000 {
		t.Error("processed data is invalid")
	}
}
