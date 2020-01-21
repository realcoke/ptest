package ptest

import (
	"sort"
)

// sortNMerge get sorted slice l and unsorted slice r
// it sorts r and merge 2 slices and return the merged slice
func sortNMerge(l []int, r []int) []int {
	lenl, lenr := len(l), len(r)
	n := lenl + lenr
	new := make([]int, n)
	sort.Ints(r)

	// merge
	li, ri, i := 0, 0, 0
	for i < n {
		if li == lenl {
			new[i] = r[ri]
			ri++
		} else if ri == lenr || l[li] < r[ri] {
			new[i] = l[li]
			li++
		} else {
			new[i] = r[ri]
			ri++
		}
		i++
	}

	return new
}

// Monitor get raw trips data of test from InputChan
// It generates Stat per second and send it to ResultChan
type Monitor struct {
	InputChan  chan *TripsOfSec
	processingStats  map[int64]*ProcessingStat
	ResultChan chan Stat
}

// Stat is statistic data for the second, the timd indicate Time variable
type Stat struct {
	TpsSuccess            int
	TpsFailure            int
	ResponseTime          int
	ResponseTime90        int
	ResponseTime95        int
	ResponseTime99        int
	FailureResponseTime   int
	FailureResponseTime90 int
	FailureResponseTime95 int
	FailureResponseTime99 int
	ErrorRate             float32
	Time                  int64
}

// ProcessingStat contains both Stat and raw data
type ProcessingStat struct {
	tripsOfSec *TripsOfSec
	Stat
}

// NewMonitor generate a Monitor, start consume and return it
func NewMonitor(inputChan chan *TripsOfSec) *Monitor {
	m := &Monitor{
		InputChan:  inputChan,
		processingStats:  map[int64]*ProcessingStat{},
		ResultChan: make(chan Stat, 1024),
	}
	go m.consume()
	return m
}

func (m *Monitor) consume() {
	for {
		// get raw data from input
		tripsOfSec, more := <-m.InputChan
		if !more {
			close(m.ResultChan)
			return
		}

		// get Processing data from map
		ps, ok := m.processingStats[tripsOfSec.Time]
		if !ok {
			// generate one if it does not exsit
			ps = &ProcessingStat{
				tripsOfSec: newTripsOfSec(tripsOfSec.Time),
			}
			ps.Time = tripsOfSec.Time
			m.processingStats[tripsOfSec.Time] = ps
		}

		// add raw trip data from input channel to processingStat
		ps.tripsOfSec.Success = sortNMerge(ps.tripsOfSec.Success, tripsOfSec.Success)
		ps.tripsOfSec.Failure = sortNMerge(ps.tripsOfSec.Failure, tripsOfSec.Failure)

		ps.TpsSuccess = len(ps.tripsOfSec.Success)
		ps.TpsFailure = len(ps.tripsOfSec.Failure)

		// calculate trip counts of 90%, 95% and 99% (bad) case
		s99 := int(float32(ps.TpsSuccess) * 0.99)
		s95 := int(float32(ps.TpsSuccess) * 0.95)
		s90 := int(float32(ps.TpsSuccess) * 0.90)

		f99 := int(float32(ps.TpsFailure) * 0.99)
		f95 := int(float32(ps.TpsFailure) * 0.95)
		f90 := int(float32(ps.TpsFailure) * 0.90)

		// calculate averate response time of each cases
		sum := 0
		for i := ps.TpsSuccess - 1; 0 <= i; i-- {
			sum += ps.tripsOfSec.Success[i]
			if i == s90 {
				ps.ResponseTime90 = sum / (ps.TpsSuccess - i)
			}
			if i == s95 {
				ps.ResponseTime95 = sum / (ps.TpsSuccess - i)
			}
			if i == s99 {
				ps.ResponseTime99 = sum / (ps.TpsSuccess - i)
			}
			if i == 0 {
				ps.ResponseTime = sum / ps.TpsSuccess
			}
		}
		sum = 0
		for i := ps.TpsFailure - 1; 0 <= i; i-- {
			sum += ps.tripsOfSec.Failure[i]
			if i == f90 {
				ps.FailureResponseTime90 = sum / (ps.TpsFailure - i)
			}
			if i == f95 {
				ps.FailureResponseTime95 = sum / (ps.TpsFailure - i)
			}
			if i == f99 {
				ps.FailureResponseTime99 = sum / (ps.TpsFailure - i)
			}
			if i == 0 {
				ps.FailureResponseTime = sum / ps.TpsFailure
			}
		}
		ps.ErrorRate = float32(ps.TpsFailure) / float32(ps.TpsSuccess+ps.TpsFailure)
		// send stat every time we get updated one
		m.ResultChan <- ps.Stat
	}
}
