package client

import (
	"testing"
)

var testNums = []struct {
	in               []float64
	expectedMean     float64
	expectedMedian   float64
	expectedVariance float64
	expectedStdDev   float64
}{
	{[]float64{}, 0, 0, 0, 0},
	{[]float64{-10, 0, 10, 20, 30}, 10, 10, 200, 14.142135623730951},
	{[]float64{8, 9, 10, 11, 12}, 10, 10, 2, 1.4142135623730951},
	{[]float64{40, 10, 20, 30, 1}, 20.20, 20, 192.16, 13.862178760930766},
	{[]float64{3, 2, 2}, 2.3333333333333335, 2, 0.22222222222222224, 0.4714045207910317},
	{[]float64{3, 2, 1, 9}, 3.75, 2.5, 9.6875, 3.112474899497183},
}

var testStatusCodes = []struct {
	in             []int
	expectedNon200 int
}{
	{[]int{200, 200, 404, 504, 200, 404, 504}, 4},
	{[]int{200, 404, 404, 504, 500}, 4},
	{[]int{405, 401, 401, 200, 408, 502, 502, 200}, 6},
}

var testPercentile = []struct {
	in         []float64
	expected95 float64
}{
	{[]float64{1, 2, 3, 4, 5, 6, 7, 8, 9}, 8},
	{[]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 1, 1, 2, 3, 4, 5, 101, 44, 33, 22}, 44},
	{[]float64{101, 202, 200, 100, 150, 144, 532, 874}, 532},
}

func TestCalcMean(t *testing.T) {
	for i, tt := range testNums {
		actual := calcMean(tt.in)
		if actual != tt.expectedMean {
			t.Errorf("test: %d, calcMean(%f): expected %f, actual %f", i+1, tt.in, tt.expectedMean, actual)
		}
	}
}

func TestCalcMedian(t *testing.T) {
	for i, tt := range testNums {
		actual := calcMedian(tt.in)
		if actual != tt.expectedMedian {
			t.Errorf("test: %d, calcMedian(%f): expected %f, actual %f", i+1, tt.in, tt.expectedMedian, actual)
		}
	}
}

func TestCalcVariance(t *testing.T) {
	for i, tt := range testNums {
		actual := calcVarience(tt.in)
		if actual != tt.expectedVariance {
			t.Errorf("test: %d, calcVariance(%f): expected %f, actual %f", i+1, tt.in, tt.expectedVariance, actual)
		}
	}
}

func TestCalcStdDev(t *testing.T) {
	for i, tt := range testNums {
		actual := calcStdDev(tt.in)
		if actual != tt.expectedStdDev {
			t.Errorf("test: %d, calcStdDev(%f): expected %f, actual %f", i+1, tt.in, tt.expectedStdDev, actual)
		}
	}
}

func Test95Percentile(t *testing.T) {
	for i, tt := range testPercentile {
		actual := calc95Percentile(tt.in)
		if actual != tt.expected95 {
			t.Errorf("test: %d, return95Percentile(%f): expected %f, actual %f", i+1, tt.in, tt.expected95, actual)
		}
	}
}

func TestFailedRequests(t *testing.T) {
	for i, tt := range testStatusCodes {
		actual := failedRequests(tt.in)
		if actual != tt.expectedNon200 {
			t.Errorf("test: %d, failedRequests(%d): expected %d, actual %d", i+1, tt.in, tt.expectedNon200, actual)
		}
	}
}
