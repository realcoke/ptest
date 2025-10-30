package ptest

import (
	"sync"
)

// ChartData represents optimized chart data with different resolutions
type ChartData struct {
	Recent   []*Stat `json:"recent"`   // Last 5 minutes, 1-second resolution
	Medium   []*Stat `json:"medium"`   // Last 30 minutes, 5-second resolution
	LongTerm []*Stat `json:"longterm"` // Full duration, 30-second resolution
}

// ChartDataManager manages chart data with automatic optimization
type ChartDataManager struct {
	recentBuffer   *CircularBuffer
	mediumBuffer   *CircularBuffer
	longTermBuffer *CircularBuffer

	maxRecentPoints   int
	maxMediumPoints   int
	maxLongTermPoints int

	lastMediumTime   int64
	lastLongTermTime int64

	mediumAccumulator   []*Stat
	longTermAccumulator []*Stat

	mutex sync.RWMutex
}

// newChartDataManager creates a new chart data manager
func newChartDataManager() *ChartDataManager {
	return &ChartDataManager{
		recentBuffer:        newCircularBuffer(300), // 5 minutes
		mediumBuffer:        newCircularBuffer(360), // 30 minutes / 5 seconds
		longTermBuffer:      newCircularBuffer(480), // 4 hours / 30 seconds
		maxRecentPoints:     300,
		maxMediumPoints:     360,
		maxLongTermPoints:   480,
		mediumAccumulator:   make([]*Stat, 0, 5),
		longTermAccumulator: make([]*Stat, 0, 30),
		mutex:               sync.RWMutex{},
	}
}

// AddDataPoint adds a new data point and handles compression
func (cdm *ChartDataManager) AddDataPoint(stat *Stat) {
	cdm.mutex.Lock()
	defer cdm.mutex.Unlock()

	// Always add to recent buffer
	cdm.recentBuffer.Add(stat)

	// Handle medium-term aggregation (5-second intervals)
	cdm.mediumAccumulator = append(cdm.mediumAccumulator, stat)
	if stat.Time >= cdm.lastMediumTime+5 || len(cdm.mediumAccumulator) >= 5 {
		if len(cdm.mediumAccumulator) > 0 {
			aggregated := cdm.aggregateStats(cdm.mediumAccumulator)
			cdm.mediumBuffer.Add(aggregated)
			cdm.lastMediumTime = stat.Time
			cdm.mediumAccumulator = cdm.mediumAccumulator[:0]
		}
	}

	// Handle long-term aggregation (30-second intervals)
	cdm.longTermAccumulator = append(cdm.longTermAccumulator, stat)
	if stat.Time >= cdm.lastLongTermTime+30 || len(cdm.longTermAccumulator) >= 30 {
		if len(cdm.longTermAccumulator) > 0 {
			aggregated := cdm.aggregateStats(cdm.longTermAccumulator)
			cdm.longTermBuffer.Add(aggregated)
			cdm.lastLongTermTime = stat.Time
			cdm.longTermAccumulator = cdm.longTermAccumulator[:0]
		}
	}
}

// GetOptimizedData returns optimized chart data
func (cdm *ChartDataManager) GetOptimizedData() *ChartData {
	cdm.mutex.RLock()
	defer cdm.mutex.RUnlock()

	return &ChartData{
		Recent:   cdm.recentBuffer.GetAll(),
		Medium:   cdm.mediumBuffer.GetAll(),
		LongTerm: cdm.longTermBuffer.GetAll(),
	}
}

// aggregateStats aggregates multiple stats into one with proper weighted averaging
func (cdm *ChartDataManager) aggregateStats(stats []*Stat) *Stat {
	if len(stats) == 0 {
		return nil
	}
	if len(stats) == 1 {
		return stats[0]
	}

	aggregated := &Stat{
		Time: stats[len(stats)-1].Time, // Use the latest time
	}

	var totalSuccess, totalFailure int
	var successRTSum, failureRTSum float64
	var successCount, failureCount int

	// Response time data for percentiles (collect raw values if available)
	var allSuccessRTs, allFailureRTs []float64

	for _, stat := range stats {
		aggregated.TpsSuccess += stat.TpsSuccess
		aggregated.TpsFailure += stat.TpsFailure
		totalSuccess += stat.SuccessCount
		totalFailure += stat.FailureCount

		// Weighted sum for response times
		if stat.SuccessCount > 0 && stat.ResponseTime > 0 {
			successRTSum += stat.ResponseTime * float64(stat.SuccessCount)
			successCount += stat.SuccessCount
		}
		if stat.FailureCount > 0 && stat.FailureResponseTime > 0 {
			failureRTSum += stat.FailureResponseTime * float64(stat.FailureCount)
			failureCount += stat.FailureCount
		}

		// For percentiles, we approximate using existing percentile values
		// This is not perfect but better than simple averaging
		if stat.SuccessCount > 0 {
			allSuccessRTs = append(allSuccessRTs,
				stat.ResponseTime, stat.ResponseTime90,
				stat.ResponseTime95, stat.ResponseTime99)
		}
		if stat.FailureCount > 0 {
			allFailureRTs = append(allFailureRTs,
				stat.FailureResponseTime, stat.FailureResponseTime90,
				stat.FailureResponseTime95, stat.FailureResponseTime99)
		}
	}

	aggregated.SuccessCount = totalSuccess
	aggregated.FailureCount = totalFailure

	// Calculate error rate
	total := totalSuccess + totalFailure
	if total > 0 {
		aggregated.ErrorRate = float64(totalFailure) / float64(total) * 100
	}

	// Calculate weighted average response times
	if successCount > 0 {
		aggregated.ResponseTime = successRTSum / float64(successCount)
		// Approximate percentiles from available data
		if len(allSuccessRTs) > 0 {
			aggregated.ResponseTime90 = calculatePercentileFloat(allSuccessRTs, 90)
			aggregated.ResponseTime95 = calculatePercentileFloat(allSuccessRTs, 95)
			aggregated.ResponseTime99 = calculatePercentileFloat(allSuccessRTs, 99)
		}
	}

	if failureCount > 0 {
		aggregated.FailureResponseTime = failureRTSum / float64(failureCount)
		// Approximate percentiles from available data
		if len(allFailureRTs) > 0 {
			aggregated.FailureResponseTime90 = calculatePercentileFloat(allFailureRTs, 90)
			aggregated.FailureResponseTime95 = calculatePercentileFloat(allFailureRTs, 95)
			aggregated.FailureResponseTime99 = calculatePercentileFloat(allFailureRTs, 99)
		}
	}

	return aggregated
}

// CircularBuffer is a circular buffer for stats
type CircularBuffer struct {
	data     []*Stat
	capacity int
	size     int
	head     int
}

// newCircularBuffer creates a new circular buffer
func newCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{
		data:     make([]*Stat, capacity),
		capacity: capacity,
		size:     0,
		head:     0,
	}
}

// Add adds an item to the buffer
func (cb *CircularBuffer) Add(item *Stat) {
	cb.data[cb.head] = item
	cb.head = (cb.head + 1) % cb.capacity

	if cb.size < cb.capacity {
		cb.size++
	}
}

// GetAll returns all items in chronological order
func (cb *CircularBuffer) GetAll() []*Stat {
	if cb.size == 0 {
		return nil
	}

	result := make([]*Stat, cb.size)

	if cb.size < cb.capacity {
		// Buffer not full yet
		copy(result, cb.data[:cb.size])
	} else {
		// Buffer is full, need to handle wrap-around
		copy(result, cb.data[cb.head:])
		copy(result[cb.capacity-cb.head:], cb.data[:cb.head])
	}

	return result
}

// Helper functions for float64 slices
func calculateMeanFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculatePercentileFloat(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Sort values for proper percentile calculation
	sorted := make([]float64, len(values))
	copy(sorted, values)

	// Simple bubble sort (sufficient for small slices)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	// Calculate percentile index
	index := (percentile / 100.0) * float64(len(sorted)-1)

	// Handle exact index
	if index == float64(int(index)) {
		return sorted[int(index)]
	}

	// Interpolate between two values
	lower := int(index)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}
