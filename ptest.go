package ptest

import "time"

// Trip is a single test data
type Trip struct {
	start   time.Time
	success bool
}

// TripsOfSec is a trip data set for the second
type TripsOfSec struct {
	Time    int64
	Success []int
	Failure []int
}

func newTripsOfSec(now int64) *TripsOfSec {
	return &TripsOfSec{
		Time:    now,
		Success: make([]int, 0),
		Failure: make([]int, 0),
	}
}

func (s *TripsOfSec) feed(success bool, responseTime int) {
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
	ResultChan chan *TripsOfSec
}

// NewCollector create a Collector and return it
func NewCollector() *Collector {
	c := &Collector{
		tripChan:   make(chan *Trip, 1024),
		ResultChan: make(chan *TripsOfSec, 1024),
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

// consume processes raw test data, trip
// it feed raw data to stat and stream stat to result channel
func (c *Collector) consume() {
	tripsOfSec := newTripsOfSec(time.Now().Unix())
	for {
		trip, more := <-c.tripChan
		if !more {
			c.publish(tripsOfSec)
			close(c.ResultChan)
			return
		}

		now := time.Now()
		nowUnix := now.Unix()
		responseTime := int(now.Sub(trip.start).Milliseconds())

		if tripsOfSec.Time != nowUnix {
			c.publish(tripsOfSec)
			tripsOfSec = newTripsOfSec(nowUnix)
		}
		tripsOfSec.feed(trip.success, responseTime)
	}
}

func (c *Collector) publish(s *TripsOfSec) {
	if len(s.Success) != 0 || len(s.Failure) != 0 {
		c.ResultChan <- s
	}
}
