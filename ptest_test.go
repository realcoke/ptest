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
		go func() {
			// 1000 TPS per 1 goroutine * 100 goroutines => 100,000 TPS
			// in 5 seconds
			for j := 0; j < 5000; j++ {
				start := time.Now()
				time.Sleep(time.Millisecond)
				collector.Report(start, true)
			}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		collector.Stop()
		t.Log("stop")
	}()

	total := 0

	for {
		s, more := <-collector.ResultChan
		if !more {
			break
		}
		total += len(s.Success)
		t.Log(s.Time, len(s.Success))
	}
	t.Log(total)
	if total != 500000 {
		t.Error("invalid count")
	}
}
