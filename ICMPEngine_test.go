package icmpengine_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/EdgeCast/icmpengine"
	hclog "github.com/hashicorp/go-hclog"
	"inet.af/netaddr"
)

//gonum.org/v1/gonum/stat

const (
	testDebugLevel     int     = 111
	setupOverheadConst float64 = 1.1 // +10%

	// fakeSuccesCst = false means actually ping over the loopback
	fakeSuccesCst bool = true

	allowMinorLoopbackPacketLossCst bool = true
)

// testsT struct defines the inputs for the tests
type testT struct {
	i           int
	IPs         []string
	count       int
	interval    time.Duration
	maxMedian   time.Duration
	successes   int
	failures    int
	fakeDrop    float64
	expected    bool
	debuglevels icmpengine.DebugLevelsT
}

// getTests returns the tests table
//
// apparently 127.0.0.2 pings now on linux, or actually anything in 127/8
//https://unix.stackexchange.com/questions/508157/how-come-one-can-successfully-ping-127-0-0-2-on-linux
func getTests(debuglevel int) (tests []testT) {

	debugLevels := icmpengine.GetDebugLevels(debuglevel)

	// note that with the race detector enabled, the max needs to be increased

	maxMedian := 20 * time.Millisecond
	if icmpengine.IsRaceEnabled {
		maxMedian = maxMedian * 10
	}

	tests = []testT{
		{
			i:           0,
			IPs:         []string{`127.0.0.1`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           1,
			IPs:         []string{`::1`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           2,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           3,
			IPs:         []string{`127.0.0.2`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           4,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       50,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   50,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           5,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       50,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   50,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           6,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       20,
			interval:    1 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   20,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           7,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       20,
			interval:    20 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   20,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           7,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       10,
			interval:    100 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
	}
	return tests
}

// getLongTests returns some long duration (1s) tests
func getLongTests(debuglevel int) (tests []testT) {

	debugLevels := icmpengine.GetDebugLevels(debuglevel)

	maxMedian := 20 * time.Millisecond
	if icmpengine.IsRaceEnabled {
		maxMedian = maxMedian * 10
	}

	tests = []testT{
		{
			i:           0,
			IPs:         []string{`127.0.0.1`},
			count:       10,
			interval:    1 * time.Second,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           0,
			IPs:         []string{`::1`},
			count:       10,
			interval:    1 * time.Second,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           0,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       10,
			interval:    1 * time.Second,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           0,
			IPs:         []string{`127.0.0.1`, `127.0.0.2`, `127.0.0.3`, `127.0.0.4`, `::1`},
			count:       10,
			interval:    1 * time.Second,
			maxMedian:   maxMedian,
			successes:   10,
			failures:    0,
			expected:    true,
			debuglevels: debugLevels,
		},
	}
	return tests
}

// getLongTests returns some long duration (1s) tests
func getTestsFakeDrop(debuglevel int) (tests []testT) {

	debugLevels := icmpengine.GetDebugLevels(debuglevel)

	// note that with the race detector enabled, the max needs to be increased

	maxMedian := 20 * time.Millisecond
	if icmpengine.IsRaceEnabled {
		maxMedian = maxMedian * 10
	}

	tests = []testT{
		{
			i:           0,
			IPs:         []string{`127.0.0.1`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			fakeDrop:    1,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           1,
			IPs:         []string{`::1`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			successes:   0,
			failures:    10,
			fakeDrop:    1,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           2,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       10,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			fakeDrop:    1,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           3,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       100,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			fakeDrop:    0.5,
			expected:    true,
			debuglevels: debugLevels,
		},
		{
			i:           3,
			IPs:         []string{`127.0.0.1`, `::1`},
			count:       100,
			interval:    10 * time.Millisecond,
			maxMedian:   maxMedian,
			fakeDrop:    0.25,
			expected:    true,
			debuglevels: debugLevels,
		},
	}
	return tests
}

// compareResults helper function
// which compares the results with the expected results
func compareResults(t *testing.T, logger hclog.Logger, i int, test testT, results icmpengine.PingerResults) {

	// Median time
	//sort.Slice(results.RTTs, func(i, j int) bool { return results.RTTs[i] < results.RTTs[j] })
	median := time.Duration(results.RTTs[int(len(results.RTTs)/2)])

	if test.expected == true {
		if median > test.maxMedian {
			t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t median:%s > test.maxMedian:%s", test.i, results.IP, median.String(), test.maxMedian.String()))
		} else {
			logger.Info(fmt.Sprintf("test:%d \t IP:%s \t median:%s < test.maxMedian:%s = good", test.i, results.IP, median.String(), test.maxMedian.String()))
		}
	}

	// Success / Failure
	if test.expected == true {
		if results.Successes != test.successes {
			if allowMinorLoopbackPacketLossCst {
				if results.Failures == 1 {
					logger.Info(fmt.Sprintf("test:%d \t IP:%s \t results.Successes:%d != test.successes:%d, but allowMinorLoopbackPacketLossCst:%t", test.i, results.IP, results.Successes, test.successes, allowMinorLoopbackPacketLossCst))
				}
			} else {
				t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t results.Successes:%d != test.successes:%d", test.i, results.IP, results.Successes, test.successes))
			}
		}

		if results.Failures != test.failures {
			if allowMinorLoopbackPacketLossCst {
				if results.Failures == 1 {
					logger.Info(fmt.Sprintf("test:%d \t IP:%s \t results.Failures:%d != test.failures:%d, but allowMinorLoopbackPacketLossCst:%t", test.i, results.IP, results.Failures, test.failures, allowMinorLoopbackPacketLossCst))
				}
			} else {
				t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t results.Failures:%d != test.failures:%d", test.i, results.IP, results.Failures, test.failures))
			}
		}

		if results.OutOfOrder != 0 {
			//t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t OutOfOrder:%d != 0", test.i, results.IP, results.OutOfOrder))
			logger.Info(fmt.Sprintf("test:%d \t IP:%s \t OutOfOrder:%d != 0", test.i, results.IP, results.OutOfOrder))
		}
	}
}

// compareResultsFakeDrop does a crude check that we did some dropping, but because we are
// performing a relatively low number of pings for the test, the run isn't long enough
// to expect 50/50 splits, so using a fudgeFactor
// At least for the fakeDrop=1 case, we can ensure no drops
func compareResultsFakeDrop(t *testing.T, logger hclog.Logger, i int, test testT, results icmpengine.PingerResults) {

	var fudgeFactor float64 = 0.5

	if test.fakeDrop == 1 {
		if results.Successes > 0 {
			t.Errorf(fmt.Sprintf("test.fakeDrop == 1 && results.Successes:%d > 0", results.Successes))
		}
		if results.Failures != int(results.Count) {
			t.Errorf(fmt.Sprintf("test.fakeDrop == 1 && results.Failures:%d != int(results.Count):%d", results.Failures, int(results.Count)))
		}
	}

	if test.fakeDrop < 1 {

		// Success / Failure
		if test.expected == true {

			expectedDrop := int(float64(test.count) * test.fakeDrop)
			expectedDropHigh := int(float64(expectedDrop) * (1 + fudgeFactor))
			expectedDropLow := int(float64(expectedDrop) * fudgeFactor)

			expectedHigh := test.count - expectedDropLow
			expectedLow := test.count - expectedDropHigh

			if results.Successes > expectedHigh {
				t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t results.Successes:%d > expectedHigh:%d", test.i, results.IP, results.Successes, expectedHigh))
			}

			if results.Failures > expectedDropHigh {
				t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t results.Failures:%d > expectedDropHigh:%d", test.i, results.IP, results.Failures, expectedDropHigh))
			}

			if results.Successes < expectedLow {
				t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t results.Successes:%d < expectedLow:%d", test.i, results.IP, results.Successes, expectedLow))
			}

			if results.Failures < expectedDropLow {
				t.Errorf(fmt.Sprintf("test:%d \t IP:%s \t results.Failures:%d < expectedDropLow:%d", test.i, results.IP, results.Successes, expectedDropLow))
			}
		}
	}
}

// TestPinger does a basic test of ICMPEngine Pinger
// It pings the loopback interfaces and checks the ping times aren't too high
// There are probably better ways to test ICMP engine
// This is testing the blocking Pinger, rather than the PingerWithStatsChannel
func TestPinger(t *testing.T) {
	logger := hclog.Default()
	logger.Info("\n\n======================================")

	debugLevel := testDebugLevel
	timeoutT := 10 * time.Millisecond
	readDeadlineT := 500 * time.Millisecond
	debugLevels := icmpengine.GetDebugLevels(debugLevel)

	doneAll := make(chan struct{}, 2)
	// no splay faster starting for testing
	ie := icmpengine.NewFullConfig(logger, doneAll, timeoutT, readDeadlineT, false, 2, 2, false, debugLevels, fakeSuccesCst)
	ie.Start()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go ie.Run(wg)

	pDone := make(chan struct{}, 2)

	tests := getTests(10)

	for i, test := range tests {
		logger.Info("======================================")
		logger.Info(fmt.Sprintf("TestPinger \t i:%d \t test.i:%d \ttest.count:%d", i, test.i, test.count))
		logger.Info(fmt.Sprintf("TestPinger i:\t%d\ttest.IPs[0]:%s", i, test.IPs))

		for j, IP := range test.IPs {

			destNetAddr, err := netaddr.ParseIP(IP)
			if err != nil {
				if test.expected != false {
					t.Errorf(fmt.Sprintf("TestPinger test netaddr.ParseIP(IP) failed:%v", err))
				} else {
					continue
				}
			}

			if testDebugLevel > 100 {
				logger.Info(fmt.Sprintf("TestPinger Pinger, index:%d \t j:%d \t%s", i, j, destNetAddr.String()))
			}
			results := ie.Pinger(destNetAddr, icmpengine.Sequence(test.count), test.interval, true, pDone)

			if testDebugLevel > 10 {
				logger.Info(fmt.Sprintf("TestPinger:[%s] \tsuccesses:%d \tfailures:%d \tooo:%d \tcount:%d", results.IP.String(), results.Successes, results.Failures, results.OutOfOrder, results.Count))
				logger.Info(fmt.Sprintf("TestPinger:[%s] \tmin:%s \tmax:%s \tmean:%s \tsum:%s \tPingerDuration:%s", results.IP.String(), results.Min.String(), results.Max.String(), results.Mean.String(), results.Sum.String(), results.PingerDuration.String()))
			}

			compareResults(t, logger, i, test, results)
		}
	}
	doneAll <- struct{}{}

	if testDebugLevel > 100 {
		logger.Info("TestPinger wg.Wait")
	}
	wg.Wait()

	if testDebugLevel > 100 {
		logger.Info("TestPinger Completed.  Bye bye")
	}
}

// TestPingerWithStatsChannel tests the non-blocking pinger
func TestPingerWithStatsChannel(t *testing.T) {
	logger := hclog.Default()
	logger.Info("\n\n######################################################")

	debugLevel := 11
	timeoutT := 10 * time.Millisecond
	readDeadlineT := 500 * time.Millisecond
	debugLevels := icmpengine.GetDebugLevels(debugLevel)

	doneAll := make(chan struct{}, 2)
	ie := icmpengine.NewFullConfig(logger, doneAll, timeoutT, readDeadlineT, false, 2, 2, false, debugLevels, fakeSuccesCst)

	pDone := make(chan struct{}, 2)

	tests := getTests(debugLevel)

	pwg := new(sync.WaitGroup)

	for i, test := range tests {

		// to test a specific test, but need to work out using subtests
		// if test.i != 6 {
		// 	continue
		// }

		logger.Info("######################################################")
		logger.Info(fmt.Sprintf("TestPingerWithStatsChannel\t i:%d \t test.i:%d", i, test.i))
		logger.Info(fmt.Sprintf("TestPingerWithStatsChannel\t i:%d \t test.IPs:%s \t len(test.IPs):%d", i, test.IPs, len(test.IPs)))
		logger.Info("######################################################")

		ie.Start()
		wg := new(sync.WaitGroup)
		wg.Add(1)
		go ie.Run(wg)

		sCh := make(chan icmpengine.PingerResults, len(test.IPs))

		for j, IP := range test.IPs {

			destNetAddr, err := netaddr.ParseIP(IP)
			if err != nil {
				if test.expected != false {
					t.Errorf(fmt.Sprintf("TestPingerWithStatsChannel test netaddr.ParseIP(IP) failed:%v", err))
				} else {
					continue
				}
			}

			if testDebugLevel > 100 {
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel Starting go PingerWithStatsChannel, index:%d \tj:%d \t %s", i, j, destNetAddr.String()))
			}
			pwg.Add(1)
			go ie.PingerWithStatsChannel(destNetAddr, icmpengine.Sequence(test.count), test.interval, true, pDone, pwg, sCh)
		}
		for j, IP := range test.IPs {

			if testDebugLevel > 100 {
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel Block on results := <-sCh, index:%d \tj:%d \t len(test.IPs):%d", i, j, len(test.IPs)))
			}

			results := <-sCh

			if testDebugLevel > 100 {
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel Received on results := <-sCh, index:%d \tj:%d \t len(test.IPs):%d", i, j, len(test.IPs)))
			}

			destNetAddr, err := netaddr.ParseIP(IP)
			if err != nil {
				if test.expected != false {
					t.Errorf(fmt.Sprintf("TestPingerWithStatsChannel test netaddr.ParseIP(IP) failed:%v", err))
				} else {
					continue
				}
			}

			// Depending on what get's pinged the results can come back in different orders
			if destNetAddr != results.IP {
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel destNetAddr[%s] != results.IP[%s], which is ok, because the order is non deterministic", destNetAddr.String(), results.IP.String()))

			}
			if testDebugLevel > 10 {
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel:[%s] \t results := <-sCh \t j:%d", results.IP.String(), j))
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel:[%s] \tsuccesses:%d \tfailures:%d \tooo:%d \tcount:%d", results.IP.String(), results.Successes, results.Failures, results.OutOfOrder, results.Count))
				logger.Info(fmt.Sprintf("TestPingerWithStatsChannel:[%s] \tmin:%s \tmax:%s \tmean:%s \tsum:%s \tPingerDuration:%s", results.IP.String(), results.Min.String(), results.Max.String(), results.Mean.String(), results.Sum.String(), results.PingerDuration.String()))
			}

			compareResults(t, logger, i, test, results)
		}
		if testDebugLevel > 100 {
			logger.Info(fmt.Sprintf("TestPingerWithStatsChannel\ti:%d, doneAll <- struct{}{}", i))
		}
		doneAll <- struct{}{}

		if testDebugLevel > 100 {
			logger.Info(fmt.Sprintf("TestPingerWithStatsChannel i:%d pwg.Wait()", i))
		}
		pwg.Wait()

		if testDebugLevel > 100 {
			logger.Info(fmt.Sprintf("TestPingerWithStatsChannel i:%d wg.Wait()", i))
		}
		wg.Wait()
	}
}

// TestRunStopLoop tests starting the 'go ie.Run()'
// and then sending done to close it down
func TestRunStopLoop(t *testing.T) {
	logger := hclog.Default()
	logger.Info("\n\n******************************************")

	timeoutT := 10 * time.Millisecond
	readDeadlineT := 500 * time.Millisecond
	debugLevels := icmpengine.GetDebugLevels(10)

	doneAll := make(chan struct{}, 2)
	ie := icmpengine.NewFullConfig(logger, doneAll, timeoutT, readDeadlineT, false, 2, 2, false, debugLevels, fakeSuccesCst)

	for i := 0; i < 10; i++ {

		ie.Start()
		wg := new(sync.WaitGroup)
		wg.Add(1)
		go ie.Run(wg)

		doneAll <- struct{}{}
		if testDebugLevel > 100 {
			logger.Info(fmt.Sprintf("TestRunStopLoop i:%d wg.Wait()", i))
		}
		wg.Wait()
	}
}

// // TestPingersShutdown is a test of closing individual pingers
// // It tests starting the 'go ie.Run()', some pingers
// // and then killing the even index numbered pingers
// // and then sending done to close everything down
// func TestPingersShutdown(t *testing.T) {

// 	debugLevel := testDebugLevel
// 	tests := getTests(debugLevel)
// 	PingersShutdown(t, tests, debugLevel)

// 	tests = getLongTests(debugLevel)
// 	PingersShutdown(t, tests, debugLevel)

// }
// func PingersShutdown(t *testing.T, tests []testT, debugLevel int) {
// 	logger := hclog.Default()
// 	logger.Info("\n\n/////////////////////////////////////////////")

// 	timeoutT := 10 * time.Millisecond
// 	readDeadlineT := 500 * time.Millisecond
// 	debugLevels := icmpengine.GetDebugLevels(debugLevel)

// 	doneAll := make(chan struct{}, 2)
// 	ie := icmpengine.NewFullConfig(logger, doneAll, timeoutT, readDeadlineT, false, 2, 2, debugLevels, fakeSuccesCst)

// 	// pDone := make(chan struct{}, 2)
// 	pDonesChs := make(map[int]map[int]chan struct{})

// 	pwg := new(sync.WaitGroup)

// 	for i, test := range tests {

// 		// to test a specific test, but need to work out using subtests
// 		// if test.i != 6 {
// 		// 	continue
// 		// }

// 		logger.Info("/////////////////////////////////////////////")
// 		logger.Info(fmt.Sprintf("PingersShutdown \t i:%d \t test.i:%d", i, test.i))
// 		logger.Info(fmt.Sprintf("PingersShutdown \t i:%d \t test.IPs:%s \t len(test.IPs):%d", i, test.IPs, len(test.IPs)))
// 		logger.Info("/////////////////////////////////////////////")

// 		pDonesChs[i] = make(map[int]chan struct{})

// 		ie.StartSplay(false) // faster starting for testing
// 		wg := new(sync.WaitGroup)
// 		wg.Add(1)
// 		go ie.Run(wg)

// 		sCh := make(chan icmpengine.PingerResults, len(test.IPs))

// 		for j, IP := range test.IPs {

// 			destNetAddr, err := netaddr.ParseIP(IP)
// 			if err != nil {
// 				if test.expected != false {
// 					t.Errorf(fmt.Sprintf("PingersShutdown test netaddr.ParseIP(IP) failed:%v", err))
// 				} else {
// 					continue
// 				}
// 			}

// 			if testDebugLevel > 100 {
// 				logger.Info(fmt.Sprintf("PingersShutdown Starting go PingerWithStatsChannel, index:%d \tj:%d \t %s", i, j, destNetAddr.String()))
// 			}
// 			pDonesChs[i][j] = make(chan struct{}, 2)

// 			pwg.Add(1)
// 			go ie.PingerWithStatsChannel(destNetAddr, icmpengine.Sequence(test.count), test.interval, true, pDonesChs[i][j], pwg, sCh)

// 			// Shutdown half of the tests midway through
// 			if i%2 == 0 {

// 				test.expected = false

// 				if testDebugLevel > 100 {
// 					logger.Info(fmt.Sprintf("PingersShutdown Nuking index:%d \t j:%d \t %s", i, j, destNetAddr.String()))
// 				}
// 				ShutdownPinger(doneAll, pDonesChs[i][j], time.Duration((float64(test.count)/2)*float64(test.interval)), logger)
// 			}
// 		}
// 		for j, IP := range test.IPs {

// 			if testDebugLevel > 100 {
// 				logger.Info(fmt.Sprintf("PingersShutdown Block on results := <-sCh, index:%d \tj:%d \t len(test.IPs):%d", i, j, len(test.IPs)))
// 			}

// 			results := <-sCh

// 			if testDebugLevel > 100 {
// 				logger.Info(fmt.Sprintf("PingersShutdown Received on results := <-sCh, index:%d \tj:%d \t len(test.IPs):%d", i, j, len(test.IPs)))
// 			}

// 			destNetAddr, err := netaddr.ParseIP(IP)
// 			if err != nil {
// 				if test.expected != false {
// 					t.Errorf(fmt.Sprintf("PingersShutdown test netaddr.ParseIP(IP) failed:%v", err))
// 				} else {
// 					continue
// 				}
// 			}

// 			// Depending on what get's pinged the results can come back in different orders
// 			if destNetAddr != results.IP {
// 				logger.Info(fmt.Sprintf("PingersShutdown destNetAddr[%s] != results.IP[%s], which is ok, because the order is non deterministic", destNetAddr.String(), results.IP.String()))

// 			}
// 			if testDebugLevel > 10 {
// 				logger.Info(fmt.Sprintf("PingersShutdown:[%s] \t results := <-sCh \t j:%d", results.IP.String(), j))
// 				logger.Info(fmt.Sprintf("PingersShutdown:[%s] \tsuccesses:%d \tfailures:%d \tooo:%d \tcount:%d", results.IP.String(), results.Successes, results.Failures, results.OutOfOrder, results.Count))
// 				logger.Info(fmt.Sprintf("PingersShutdown:[%s] \tmin:%s \tmax:%s \tmean:%s \tsum:%s \tPingerDuration:%s", results.IP.String(), results.Min.String(), results.Max.String(), results.Mean.String(), results.Sum.String(), results.PingerDuration.String()))
// 			}

// 			compareResults(t, logger, i, test, results)
// 		}
// 		if testDebugLevel > 100 {
// 			logger.Info(fmt.Sprintf("PingersShutdown\ti:%d, doneAll <- struct{}{}", i))
// 		}
// 		doneAll <- struct{}{}

// 		if testDebugLevel > 100 {
// 			logger.Info(fmt.Sprintf("PingersShutdown i:%d pwg.Wait()", i))
// 		}
// 		pwg.Wait()

// 		if testDebugLevel > 100 {
// 			logger.Info(fmt.Sprintf("PingersShutdown i:%d wg.Wait()", i))
// 		}
// 		wg.Wait()
// 	}
// }

// // ShutdownPinger is a helper function for TestPingersShutdown
// // Simply sleeps and sends done, but it can handle getting an allDone signal also
// func ShutdownPinger(allDone <-chan struct{}, done chan<- struct{}, sleep time.Duration, logger hclog.Logger) {
// 	if testDebugLevel > 100 {
// 		logger.Info(fmt.Sprintf("ShutdownPinger sleeping for:%s", sleep))
// 	}
// 	select {
// 	case <-time.After(sleep):
// 		if testDebugLevel > 100 {
// 			logger.Info("ShutdownPinger wakes up")
// 		}
// 	case <-allDone:
// 		if testDebugLevel > 10 {
// 			logger.Info("ShutdownPinger <-allDone")
// 		}

// 	}
// 	if testDebugLevel > 100 {
// 		logger.Info("ShutdownPinger done <- struct{}{}")
// 	}
// 	done <- struct{}{}
// }

// TestPinger does a basic test of ICMPEngine Pinger
// It pings the loopback interfaces and checks the ping times aren't too high
// There are probably better ways to test ICMP engine
// This is testing the blocking Pinger, rather than the PingerWithStatsChannel
func TestPingerFakeDrop(t *testing.T) {
	logger := hclog.Default()
	logger.Info("\n\n(((((((((((((((((((((((((((((((((((((((((((((((((")

	debugLevel := testDebugLevel
	timeoutT := 10 * time.Millisecond
	readDeadlineT := 500 * time.Millisecond
	debugLevels := icmpengine.GetDebugLevels(debugLevel)

	doneAll := make(chan struct{}, 2)
	ie := icmpengine.NewFullConfig(logger, doneAll, timeoutT, readDeadlineT, false, 2, 2, false, debugLevels, fakeSuccesCst)
	ie.Start()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go ie.Run(wg)

	pDone := make(chan struct{}, 2)

	tests := getTestsFakeDrop(10)

	for i, test := range tests {
		logger.Info("(((((((((((((((((((((((((((((((((((((((((((((((((")
		logger.Info(fmt.Sprintf("TestPinger \t i:%d \t test.i:%d \ttest.count:%d", i, test.i, test.count))
		logger.Info(fmt.Sprintf("TestPinger i:\t%d\ttest.IPs[0]:%s", i, test.IPs))

		// Temp hack to not drop
		//test.fakeDrop = 0

		for j, IP := range test.IPs {

			destNetAddr, err := netaddr.ParseIP(IP)
			if err != nil {
				if test.expected != false {
					t.Errorf(fmt.Sprintf("TestPinger test netaddr.ParseIP(IP) failed:%v", err))
				} else {
					continue
				}
			}

			if testDebugLevel > 100 {
				logger.Info(fmt.Sprintf("TestPinger Pinger, index:%d \t j:%d \t%s", i, j, destNetAddr.String()))
			}
			results := ie.PingerConfig(destNetAddr, icmpengine.Sequence(test.count), test.interval, true, pDone, test.fakeDrop)

			if testDebugLevel > 10 {
				logger.Info(fmt.Sprintf("TestPinger:[%s] \tsuccesses:%d \tfailures:%d \tooo:%d \tcount:%d", results.IP.String(), results.Successes, results.Failures, results.OutOfOrder, results.Count))
				logger.Info(fmt.Sprintf("TestPinger:[%s] \tmin:%s \tmax:%s \tmean:%s \tsum:%s \tPingerDuration:%s", results.IP.String(), results.Min.String(), results.Max.String(), results.Mean.String(), results.Sum.String(), results.PingerDuration.String()))
			}

			compareResultsFakeDrop(t, logger, i, test, results)
		}
	}
	doneAll <- struct{}{}

	if testDebugLevel > 100 {
		logger.Info("TestPinger wg.Wait")
	}
	wg.Wait()

	if testDebugLevel > 100 {
		logger.Info("TestPinger Completed.  Bye bye")
	}
}

// TestPingerFakeSuccess is a test of many target IPs with fake success = true
func TestPingerFakeSuccess(t *testing.T) {
	logger := hclog.Default()
	logger.Info("\n\n$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$")

	fakeSuccess := true
	debugLevel := testDebugLevel
	timeoutT := 10 * time.Millisecond
	readDeadlineT := 500 * time.Millisecond
	debugLevels := icmpengine.GetDebugLevels(debugLevel)
	start := 1
	max := 3
	count := 10
	interval := 1 * time.Microsecond

	doneAll := make(chan struct{}, 2)
	ie := icmpengine.NewFullConfig(logger, doneAll, timeoutT, readDeadlineT, false, 2, 2, false, debugLevels, fakeSuccess)
	ie.Start()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go ie.Run(wg)

	pDone := make(chan struct{}, 2)
	pwg := new(sync.WaitGroup)
	sCh := make(chan icmpengine.PingerResults, 4*(max-start))

	i := 0
	for a := start; a < max; a++ {
		for b := start; b < max; b++ {
			for c := start; c < max; c++ {
				for d := start; d < max; d++ {

					ip := fmt.Sprintf("%d.%d.%d.%d", a, b, c, d)
					destNetAddr, err := netaddr.ParseIP(ip)
					if err != nil {
						t.Errorf(fmt.Sprintf("TestPingerFakeSuccess netaddr.ParseIP(IP):%s failed:%v", ip, err))
					}
					pwg.Add(1)
					go ie.PingerWithStatsChannel(destNetAddr, icmpengine.Sequence(count), interval, true, pDone, pwg, sCh)
					logger.Info(fmt.Sprintf("TestPingerFakeSuccess go ie.PingerWithStatsChannel \t ip:%s \t count:%d \t interval:%s \t i:%d", ip, count, interval, i))

					i++
				}
			}
		}
	}

	logger.Info("\n\n$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$\n\n")

	j := 0
	for a := start; a < max; a++ {
		for b := start; b < max; b++ {
			for c := start; c < max; c++ {
				for d := start; d < max; d++ {
					results := <-sCh
					if testDebugLevel > 10 {
						logger.Info(fmt.Sprintf("TestPingerFakeSuccess:\t j:%d \t results := <-sCh \t a:%d \t b:%d \t c:%d \t d:%d", j, a, b, c, d))
						logger.Info(fmt.Sprintf("TestPingerFakeSuccess:\t [%s] \t successes:%d \tfailures:%d \tooo:%d \tcount:%d", results.IP.String(), results.Successes, results.Failures, results.OutOfOrder, results.Count))
						logger.Info(fmt.Sprintf("TestPingerFakeSuccess:\t [%s] \t min:%s \tmax:%s \tmean:%s \tsum:%s \tPingerDuration:%s", results.IP.String(), results.Min.String(), results.Max.String(), results.Mean.String(), results.Sum.String(), results.PingerDuration.String()))
					}
				}
			}
		}
	}

	if testDebugLevel > 100 {
		logger.Info("TestPingerFakeSuccess \t doneAll <- struct{}{}")
	}
	doneAll <- struct{}{}

	if testDebugLevel > 100 {
		logger.Info("TestPingerFakeSuccess pwg.Wait()")
	}
	pwg.Wait()

	if testDebugLevel > 100 {
		logger.Info("TestPingerFakeSuccess wg.Wait()")
	}
	wg.Wait()

	if testDebugLevel > 100 {
		logger.Info("TestPingerFakeSuccess complete")
	}
}

// TestIsRace is a tiny function to show the irace.go and norace.go functionality
func TestIsRace(t *testing.T) {
	logger := hclog.Default()
	logger.Info(fmt.Sprintf("\n\nTestIsRace: %t", icmpengine.IsRaceEnabled))
}

// https://stackoverflow.com/questions/44944959/how-can-i-check-if-the-race-detector-is-enabled-at-runtime
// das@das-dell5580:~/go/src/gitlab.edgecastcdn.net/dseddon/icmpengine$ go test -race --run TestIsRace
// 2021-06-28T17:38:43.806-0700 [INFO]

// TestIsRace: true
// PASS
// ok  	gitlab.edgecastcdn.net/dseddon/icmpengine	0.023s
// das@das-dell5580:~/go/src/gitlab.edgecastcdn.net/dseddon/icmpengine$ go test --run TestIsRace
// 2021-06-28T17:38:51.603-0700 [INFO]

// TestIsRace: false
// PASS
// ok  	gitlab.edgecastcdn.net/dseddon/icmpengine	0.002s
