// Copyright 2021 Edgecast Inc

package icmpengine

import (
	"fmt"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
)

const (
	testDebugLevel int = 11
)

// testsT struct defines the inputs for the tests
type rTestT struct {
	i        float32
	tiar     int
	low      int
	medium   int
	high     int
	mLow     float64
	mMedium  float64
	mHigh    float64
	result   float64
	expected bool
}

// TestTiarCalculator tests receiver.go tiarCalculator function
func TestTiarCalculator(t *testing.T) {
	var tests = []rTestT{
		{0.10, 0, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, 1, true},
		{0.20, 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, 1, true},
		{0.30, tiarLow - 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, 1, true},
		{0.40, 100, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiHigh, true},
		{0.50, 1000, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiHigh, true},
		//
		{1.10, tiarLow, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiLow, true},
		{1.20, tiarLow + 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiLow, true},
		//
		{2.10, tiarMedium - 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiLow, true},
		{2.20, tiarMedium, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiMedium, true},
		{2.30, tiarMedium + 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiMedium, true},
		//
		{3.10, tiarHigh - 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiMedium, true},
		{3.20, tiarHigh, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiHigh, true},
		{3.30, tiarHigh + 1, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh, multiHigh, true},
		//
		{4.00, 0, 10, 20, 30, 1, 2, 3, 1, true},
		{4.10, 1, 10, 20, 30, 1, 2, 3, 1, true},
		{4.20, 2, 10, 20, 30, 1, 2, 3, 1, true},
		{4.30, 10, 10, 20, 30, 1, 2, 3, 1, true},
		{4.40, 19, 10, 20, 30, 1, 2, 3, 1, true},
		{4.50, 20, 10, 20, 30, 1, 2, 3, 2, true},
		{4.60, 29, 10, 20, 30, 1, 2, 3, 2, true},
		{4.70, 30, 10, 20, 30, 1, 2, 3, 3, true},
		{4.80, 31, 10, 20, 30, 1, 2, 3, 3, true},
		//
		{5.00, 0, 100, 200, 300, 2, 3, 4, 1, true},
		{5.10, 1, 100, 200, 300, 2, 3, 4, 1, true},
		{5.20, 2, 100, 200, 300, 2, 3, 4, 1, true},
		{5.30, 100, 100, 200, 300, 2, 3, 4, 2, true},
		{5.40, 200, 100, 200, 300, 2, 3, 4, 3, true},
		{5.50, 300, 100, 200, 300, 2, 3, 4, 4, true},
	}

	logger := hclog.Default()
	if testDebugLevel > 100 {
		logger.Info("\n\n======================================")
	}

	for i, test := range tests {
		if testDebugLevel > 100 {
			logger.Info("======================================")
		}
		if testDebugLevel > 100 {
			logger.Info(fmt.Sprintf("TesttimeoutsInARowCalculator \t i:%d \t test.i:%.2f \t test.result:%.2f", i, test.i, test.result))
		}

		result := tiarCalculator(test.tiar, test.low, test.medium, test.high, test.mLow, test.mMedium, test.mHigh)

		if result != test.result {
			t.Errorf(fmt.Sprintf("TesttimeoutsInARowCalculator\ti:%d\ttest.i:%f", i, test.i))
		}

		if testDebugLevel > 10 {
			logger.Info(fmt.Sprintf("TesttimeoutsInARowCalculator \t i:%d \t test.i:%.2f \t test.result:%.2f \t result:%.2f", i, test.i, test.result, result))
		}

		if testDebugLevel > 100 {
			logger.Info("======================================")
		}
	}
}
