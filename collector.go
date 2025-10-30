package ptest

import (
	"sync/atomic"
	"time"
)

// Trip represents a single test result
type Trip struct {
	StartTime time.Time
	Success   bool
}

// TripsOfSec contains all trips within one second
type TripsOfSec struct {
	Time     int64
	Success  []int
	Failures []int
}

// DataCollector collects raw test data and aggregates by second
type DataCollector struct {
	tripChan   chan *Trip
	ResultChan chan *TripsOfSec
	totalReqs  int64
	isRunning  bool
}

// newDataCollector creates a new data collector
func newDataCollector() *DataCollector {
	dc := &DataCollector{
		tripChan:   make(chan *Trip, 2048),
		ResultChan: make(chan *TripsOfSec, 1024),
		isRunning:  true,
	}

	go dc.consume()
	return dc
}

// Report reports a single test result
func (dc *DataCollector) Report(start time.Time, success bool) {
	if !dc.isRunning {
		return
	}

	atomic.AddInt64(&dc.totalReqs, 1)

	trip := &Trip{
		StartTime: start,
		Success:   success,
	}

	select {
	case dc.tripChan <- trip:
		// Successfully sent
	default:
		// Channel full, drop the data (or implement backpressure)
	}
}

// Stop stops the data collector
func (dc *DataCollector) Stop() {
	dc.isRunning = false
	close(dc.tripChan)
}

// GetTotalRequests returns total number of requests processed
func (dc *DataCollector) GetTotalRequests() int64 {
	return atomic.LoadInt64(&dc.totalReqs)
}

// consume processes raw trips and groups them by second
func (dc *DataCollector) consume() {
	defer close(dc.ResultChan)

	var currentSec *TripsOfSec

	for trip := range dc.tripChan {
		now := time.Now()
		responseTime := int(now.Sub(trip.StartTime).Milliseconds())
		currentSecond := now.Unix()

		// Check if we need to create new second bucket
		if currentSec == nil || currentSec.Time != currentSecond {
			// Publish previous second if it exists
			if currentSec != nil {
				dc.publish(currentSec)
			}

			// Create new second bucket
			currentSec = &TripsOfSec{
				Time:     currentSecond,
				Success:  make([]int, 0),
				Failures: make([]int, 0),
			}
		}

		// Add response time to appropriate bucket
		if trip.Success {
			currentSec.Success = append(currentSec.Success, responseTime)
		} else {
			currentSec.Failures = append(currentSec.Failures, responseTime)
		}
	}

	// Publish last second if exists
	if currentSec != nil {
		dc.publish(currentSec)
	}
}

// publish sends TripsOfSec to result channel
func (dc *DataCollector) publish(trips *TripsOfSec) {
	if len(trips.Success) > 0 || len(trips.Failures) > 0 {
		select {
		case dc.ResultChan <- trips:
			// Successfully sent
		default:
			// Channel full, skip this data point
		}
	}
}
