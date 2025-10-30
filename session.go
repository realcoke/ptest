package ptest

import (
	"sync"
	"time"
)

// SessionStatus represents the status of a test session
type SessionStatus string

const (
	StatusIdle    SessionStatus = "idle"
	StatusRunning SessionStatus = "running"
	StatusStopped SessionStatus = "stopped"
)

// CumulativeStats holds cumulative statistics for weighted averaging
type CumulativeStats struct {
	TotalSuccessRT float64
	TotalFailureRT float64
	TotalSuccess   int64
	TotalFailure   int64
	mutex          sync.RWMutex
}

// TestSession represents a single test execution session
type TestSession struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	StartTime time.Time     `json:"start_time"`
	EndTime   *time.Time    `json:"end_time,omitempty"`
	Status    SessionStatus `json:"status"`

	dataCollector *DataCollector
	aggregator    *DataAggregator
	chartManager  *ChartDataManager

	// Cumulative statistics for accurate averaging
	cumulativeStats *CumulativeStats

	statsChan chan *Stat
	mutex     sync.RWMutex
}

// newTestSession creates a new test session
func newTestSession(id, name string) *TestSession {
	session := &TestSession{
		ID:              id,
		Name:            name,
		StartTime:       time.Now(),
		Status:          StatusIdle,
		statsChan:       make(chan *Stat, 1000),
		cumulativeStats: &CumulativeStats{},
		mutex:           sync.RWMutex{},
	}

	session.dataCollector = newDataCollector()
	session.aggregator = newDataAggregator()
	session.chartManager = newChartDataManager()

	return session
}

// start starts the test session
func (ts *TestSession) start() {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	if ts.Status == StatusRunning {
		return
	}

	ts.Status = StatusRunning
	ts.StartTime = time.Now()
	ts.EndTime = nil

	// Reset cumulative stats for new session
	ts.cumulativeStats.mutex.Lock()
	ts.cumulativeStats.TotalSuccessRT = 0
	ts.cumulativeStats.TotalFailureRT = 0
	ts.cumulativeStats.TotalSuccess = 0
	ts.cumulativeStats.TotalFailure = 0
	ts.cumulativeStats.mutex.Unlock()

	// Start data processing pipeline
	go ts.processData()
}

// stop stops the test session
func (ts *TestSession) stop() {
	ts.mutex.Lock()
	defer ts.mutex.Unlock()

	if ts.Status != StatusRunning {
		return
	}

	ts.Status = StatusStopped
	now := time.Now()
	ts.EndTime = &now

	// Stop data collector
	ts.dataCollector.Stop()
}

// Report reports a test result
func (ts *TestSession) Report(start time.Time, success bool) {
	if ts.Status != StatusRunning {
		return
	}

	ts.dataCollector.Report(start, success)
}

// processData processes collected data through the pipeline
func (ts *TestSession) processData() {
	// Connect data collector -> aggregator -> chart manager
	go ts.aggregator.Process(ts.dataCollector.ResultChan, ts.statsChan)

	// Process stats for chart optimization and cumulative tracking
	for stat := range ts.statsChan {
		ts.chartManager.AddDataPoint(stat)
		ts.updateCumulativeStats(stat)
	}
}

// updateCumulativeStats updates cumulative statistics for weighted averaging
func (ts *TestSession) updateCumulativeStats(stat *Stat) {
	ts.cumulativeStats.mutex.Lock()
	defer ts.cumulativeStats.mutex.Unlock()

	// Add weighted response times (response_time * count)
	if stat.SuccessCount > 0 && stat.ResponseTime > 0 {
		ts.cumulativeStats.TotalSuccessRT += stat.ResponseTime * float64(stat.SuccessCount)
		ts.cumulativeStats.TotalSuccess += int64(stat.SuccessCount)
	}

	if stat.FailureCount > 0 && stat.FailureResponseTime > 0 {
		ts.cumulativeStats.TotalFailureRT += stat.FailureResponseTime * float64(stat.FailureCount)
		ts.cumulativeStats.TotalFailure += int64(stat.FailureCount)
	}
}

// GetCumulativeAvgResponseTime returns overall weighted average response time
func (ts *TestSession) GetCumulativeAvgResponseTime() float64 {
	ts.cumulativeStats.mutex.RLock()
	defer ts.cumulativeStats.mutex.RUnlock()

	totalRequests := ts.cumulativeStats.TotalSuccess + ts.cumulativeStats.TotalFailure
	if totalRequests == 0 {
		return 0
	}

	totalWeightedRT := ts.cumulativeStats.TotalSuccessRT + ts.cumulativeStats.TotalFailureRT
	return totalWeightedRT / float64(totalRequests)
}

// GetCumulativeErrorRate returns overall error rate
func (ts *TestSession) GetCumulativeErrorRate() float64 {
	ts.cumulativeStats.mutex.RLock()
	defer ts.cumulativeStats.mutex.RUnlock()

	totalRequests := ts.cumulativeStats.TotalSuccess + ts.cumulativeStats.TotalFailure
	if totalRequests == 0 {
		return 0
	}

	return float64(ts.cumulativeStats.TotalFailure) / float64(totalRequests) * 100
}

// GetOptimizedChartData returns optimized chart data
func (ts *TestSession) GetOptimizedChartData() *ChartData {
	return ts.chartManager.GetOptimizedData()
}

// GetStats returns current session statistics
func (ts *TestSession) GetStats() *SessionStats {
	ts.mutex.RLock()
	defer ts.mutex.RUnlock()

	stats := &SessionStats{
		SessionID:           ts.ID,
		SessionName:         ts.Name,
		Status:              ts.Status,
		StartTime:           ts.StartTime,
		EndTime:             ts.EndTime,
		Duration:            ts.getDuration(),
		TotalRequests:       ts.dataCollector.GetTotalRequests(),
		CumulativeAvgRT:     ts.GetCumulativeAvgResponseTime(),
		CumulativeErrorRate: ts.GetCumulativeErrorRate(),
	}

	if ts.aggregator != nil {
		stats.CurrentStat = ts.aggregator.GetCurrentStat()
	}

	return stats
}

// getDuration calculates session duration
func (ts *TestSession) getDuration() time.Duration {
	if ts.EndTime != nil {
		return ts.EndTime.Sub(ts.StartTime)
	}
	if ts.Status == StatusRunning {
		return time.Since(ts.StartTime)
	}
	return 0
}

// SessionStats contains session statistics
type SessionStats struct {
	SessionID           string        `json:"session_id"`
	SessionName         string        `json:"session_name"`
	Status              SessionStatus `json:"status"`
	StartTime           time.Time     `json:"start_time"`
	EndTime             *time.Time    `json:"end_time,omitempty"`
	Duration            time.Duration `json:"duration"`
	TotalRequests       int64         `json:"total_requests"`
	CumulativeAvgRT     float64       `json:"cumulative_avg_rt"`
	CumulativeErrorRate float64       `json:"cumulative_error_rate"`
	CurrentStat         *Stat         `json:"current_stat,omitempty"`
}
