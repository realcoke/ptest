package ptest

import (
	"sort"
	"sync"
)

// Stat represents aggregated statistics for a time period
type Stat struct {
	Time                  int64   `json:"Time"`
	TpsSuccess            float64 `json:"TpsSuccess"`
	TpsFailure            float64 `json:"TpsFailure"`
	ResponseTime          float64 `json:"ResponseTime"`
	ResponseTime90        float64 `json:"ResponseTime90"`
	ResponseTime95        float64 `json:"ResponseTime95"`
	ResponseTime99        float64 `json:"ResponseTime99"`
	FailureResponseTime   float64 `json:"FailureResponseTime"`
	FailureResponseTime90 float64 `json:"FailureResponseTime90"`
	FailureResponseTime95 float64 `json:"FailureResponseTime95"`
	FailureResponseTime99 float64 `json:"FailureResponseTime99"`
	ErrorRate             float64 `json:"ErrorRate"`
	SuccessCount          int     `json:"SuccessCount"`
	FailureCount          int     `json:"FailureCount"`
}

// DataAggregator processes TripsOfSec and generates statistics
type DataAggregator struct {
	currentStat *Stat
	mutex       sync.RWMutex
}

// newDataAggregator creates a new data aggregator
func newDataAggregator() *DataAggregator {
	return &DataAggregator{
		mutex: sync.RWMutex{},
	}
}

// Process processes TripsOfSec data and outputs Stat data
func (da *DataAggregator) Process(inputChan chan *TripsOfSec, outputChan chan *Stat) {
	defer close(outputChan)

	for trips := range inputChan {
		stat := da.calculateStat(trips)

		da.mutex.Lock()
		da.currentStat = stat
		da.mutex.Unlock()

		select {
		case outputChan <- stat:
			// Successfully sent
		default:
			// Channel full, skip this stat
		}
	}
}

// GetCurrentStat returns the current stat
func (da *DataAggregator) GetCurrentStat() *Stat {
	da.mutex.RLock()
	defer da.mutex.RUnlock()
	return da.currentStat
}

// calculateStat calculates statistics from TripsOfSec
func (da *DataAggregator) calculateStat(trips *TripsOfSec) *Stat {
	stat := &Stat{
		Time:         trips.Time,
		SuccessCount: len(trips.Success),
		FailureCount: len(trips.Failures),
	}

	// Calculate TPS
	stat.TpsSuccess = float64(len(trips.Success))
	stat.TpsFailure = float64(len(trips.Failures))

	// Calculate error rate
	totalCount := len(trips.Success) + len(trips.Failures)
	if totalCount > 0 {
		stat.ErrorRate = float64(len(trips.Failures)) / float64(totalCount) * 100
	}

	// Calculate response time statistics for successful requests
	if len(trips.Success) > 0 {
		stat.ResponseTime = calculateMean(trips.Success)
		stat.ResponseTime90 = calculatePercentile(trips.Success, 90)
		stat.ResponseTime95 = calculatePercentile(trips.Success, 95)
		stat.ResponseTime99 = calculatePercentile(trips.Success, 99)
	}

	// Calculate response time statistics for failed requests
	if len(trips.Failures) > 0 {
		stat.FailureResponseTime = calculateMean(trips.Failures)
		stat.FailureResponseTime90 = calculatePercentile(trips.Failures, 90)
		stat.FailureResponseTime95 = calculatePercentile(trips.Failures, 95)
		stat.FailureResponseTime99 = calculatePercentile(trips.Failures, 99)
	}

	return stat
}

// calculateMean calculates the mean of a slice of integers
func calculateMean(values []int) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0
	for _, v := range values {
		sum += v
	}
	return float64(sum) / float64(len(values))
}

// calculatePercentile calculates the specified percentile of a slice of integers
func calculatePercentile(values []int, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Create a copy and sort it
	sorted := make([]int, len(values))
	copy(sorted, values)
	sort.Ints(sorted)

	// Calculate percentile index
	index := (percentile / 100.0) * float64(len(sorted)-1)

	// Handle exact index
	if index == float64(int(index)) {
		return float64(sorted[int(index)])
	}

	// Interpolate between two values
	lower := int(index)
	upper := lower + 1
	weight := index - float64(lower)

	return float64(sorted[lower])*(1-weight) + float64(sorted[upper])*weight
}
