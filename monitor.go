package ptest

import (
	"sort"
	"time"
)

func sortNMerge(l []int, r []int) []int {
	lenl, lenr := len(l), len(r)
	n := lenl + lenr
	new := make([]int, n)
	sort.Ints(r)

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

type Monitor struct {
	InputChan  chan *Stat
	statViews  map[int64]*StatView
	ResultChan chan StatReport
}

type StatReport struct {
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

type StatView struct {
	stat *Stat
	StatReport
}

func NewMonitor(inputChan chan *Stat) *Monitor {
	m := &Monitor{
		InputChan:  inputChan,
		statViews:  map[int64]*StatView{},
		ResultChan: make(chan StatReport, 1024),
	}
	go m.consume()
	return m
}

func (m *Monitor) consume() {
	for {
		s, more := <-m.InputChan
		if !more {
			close(m.ResultChan)
			return
		}
		sv, ok := m.statViews[s.Time]
		if !ok {
			sv = &StatView{
				stat: newStat(time.Now().Unix()),
			}
			m.statViews[s.Time] = sv
		}
		sv.Time = s.Time

		sv.stat.Success = sortNMerge(sv.stat.Success, s.Success)
		sv.stat.Failure = sortNMerge(sv.stat.Failure, s.Failure)

		sv.TpsSuccess = len(sv.stat.Success)
		sv.TpsFailure = len(sv.stat.Failure)

		s99 := int(float32(sv.TpsSuccess) * 0.99)
		s95 := int(float32(sv.TpsSuccess) * 0.95)
		s90 := int(float32(sv.TpsSuccess) * 0.90)

		f99 := int(float32(sv.TpsFailure) * 0.99)
		f95 := int(float32(sv.TpsFailure) * 0.95)
		f90 := int(float32(sv.TpsFailure) * 0.90)

		sum := 0

		for i := sv.TpsSuccess - 1; 0 <= i; i-- {
			sum += sv.stat.Success[i]
			if i == s90 {
				sv.ResponseTime90 = sum / (sv.TpsSuccess - i)
			}
			if i == s95 {
				sv.ResponseTime95 = sum / (sv.TpsSuccess - i)
			}
			if i == s99 {
				sv.ResponseTime99 = sum / (sv.TpsSuccess - i)
			}
			if i == 0 {
				sv.ResponseTime = sum / sv.TpsSuccess
			}
		}
		sum = 0
		for i := sv.TpsFailure - 1; 0 <= i; i-- {
			sum += sv.stat.Failure[i]
			if i == f90 {
				sv.FailureResponseTime90 = sum / (sv.TpsFailure - i)
			}
			if i == f95 {
				sv.FailureResponseTime95 = sum / (sv.TpsFailure - i)
			}
			if i == f99 {
				sv.FailureResponseTime99 = sum / (sv.TpsFailure - i)
			}
			if i == 0 {
				sv.FailureResponseTime = sum / sv.TpsFailure
			}
		}
		sv.ErrorRate = float32(sv.TpsFailure) / float32(sv.TpsSuccess+sv.TpsFailure)
		m.ResultChan <- sv.StatReport
	}
}
