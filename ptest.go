package ptest

import "time"

// Trip is a single test data
type Trip struct {
	start   time.Time
	success bool
}

// Stat is a statistic of one second
type Stat struct {
	Time    int64
	Success []int
	Failure []int
}

func newStat(now int64) *Stat {
	return &Stat{
		Time:    now,
		Success: make([]int, 0),
		Failure: make([]int, 0),
	}
}

func (s *Stat) feed(success bool, responseTime int) {
	if success {
		s.Success = append(s.Success, responseTime)
	} else {
		s.Failure = append(s.Failure, responseTime)
	}

}

// Collector collect test data
// and write statistics of test to ResultChan
type Collector struct {
	tripChan   chan *Trip
	ResultChan chan *Stat
}

// NewCollector create a Collector and return it
func NewCollector() *Collector {
	c := &Collector{
		tripChan:   make(chan *Trip, 1024),
		ResultChan: make(chan *Stat, 1024),
	}
	go c.consume()
	return c
}

// Stop stops the collector routine
func (c *Collector) Stop() {
	close(c.tripChan)
}

// Report gets single test data and processes it
func (c *Collector) Report(start time.Time, success bool) {
	trip := &Trip{
		start:   start,
		success: success,
	}
	c.tripChan <- trip
}

// consume is a routine to process test data
func (c *Collector) consume() {
	s := newStat(time.Now().Unix())
	for {
		trip, more := <-c.tripChan
		if !more {
			c.publish(s)
			close(c.ResultChan)
			return
		}

		now := time.Now()
		nowUnix := now.Unix()
		responseTime := int(now.Sub(trip.start).Milliseconds())

		if s.Time != nowUnix {
			c.publish(s)
			s = newStat(nowUnix)
		}
		s.feed(trip.success, responseTime)
	}
}

func (c *Collector) publish(s *Stat) {
	if len(s.Success) != 0 || len(s.Failure) != 0 {
		c.ResultChan <- s
	}
}
