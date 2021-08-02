package icmpengine

import (
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"inet.af/netaddr"

	"container/list"
	"log"
	"net"
	"syscall"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

const (
	PingerFractionModulo = 10

	PdebugLevel = 111
)

type PingerResults struct {
	IP             netaddr.IP
	Successes      int
	Failures       int
	OutOfOrder     int
	RTTs           []time.Duration
	Count          int
	Min            time.Duration
	Max            time.Duration
	Mean           time.Duration
	Variance       time.Duration
	Sum            time.Duration
	PingerDuration time.Duration
}

// PingerWithStatsChannel is the Pinger which sends stats on the output channel, rather than returning the values
func (ie *ICMPEngine) PingerWithStatsChannel(IP netaddr.IP, packets Sequence, interval time.Duration, sortRTTs bool, DoneCh chan struct{}, wg *sync.WaitGroup, pingerResultsCh chan<- PingerResults) {

	defer wg.Done()
	if ie.Pingers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("PingerWithStatsChannel started:\t%s", IP.String()))
	}

	results := ie.Pinger(IP, packets, interval, sortRTTs, DoneCh)

	if ie.Pingers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("PingerWithStatsChannel recieved results, sending on channel:\t%s", IP.String()))
	}
	pingerResultsCh <- results

	if ie.Pingers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("PingerWithStatsChannel complete:\t%s", IP.String()))
	}
}

// Pinger calls PingerConfig with:
// - zero (0) probability of drop,
// - no fake success
func (ie *ICMPEngine) Pinger(IP netaddr.IP, packets Sequence, interval time.Duration, sortRTTs bool, DoneCh chan struct{}) (results PingerResults) {
	results = ie.PingerConfig(IP, packets, interval, sortRTTs, DoneCh, 0)
	return
}

// PingerWithDropProb is primarily responsible for WriteTo-ing ICMP messages to a socket
// This is using NonPrivilegedPing ICMP sockets
//
// IPPROTO_ICMP sockets which are NonPrivilegedPing
// https://lwn.net/Articles/422330/
//
// Hash + DLL
// www.cs.columbia.edu/~nahum/w6998/papers/sosp87-timing-wheels.pdf
//
// https://pkg.go.dev/inet.af/netaddr

// Welford's math stolen from https://pkg.go.dev/github.com/eclesh/welford
// Welford's one-pass algorithm for computing the mean and variance
// of a set of numbers. For more information see Knuth (TAOCP Vol 2, 3rd ed, pg 232)
func (ie *ICMPEngine) PingerConfig(IP netaddr.IP, packets Sequence, interval time.Duration, sortRTTs bool, DoneCh chan struct{}, dropProb float64) (results PingerResults) {

	if ie.Pingers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Pinger started:\t[%s]", IP.String()))
	}

	var proto Protocol
	if IP.Is4() {
		proto = Protocol(4)
	} else {
		if IP.Is6() {
			proto = Protocol(6)
		}
	}

	if ie.Pingers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Pinger [%s] Trying to acquire lock at start", IP.String()))
	}

	successCh := make(chan PingSuccess, int(packets))
	expiredCh := make(chan PingExpired, int(packets))

	ie.Lock()
	fakeSuccess := ie.Expirers.FakeSuccess
	socket := ie.Sockets.Sockets[proto]
	id := ie.PID
	ie.Pingers.SuccessChs[IP] = successCh
	ie.Pingers.ExpiredChs[IP] = expiredCh
	ie.Pingers.DonesChs[IP] = DoneCh
	pingersAllDone := ie.Pingers.DoneCh
	ie.Unlock()

	if ie.Pingers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Pinger [%s] Unlocked", IP.String()))
	}

	results.IP = IP
	results.RTTs = make([]time.Duration, int(packets))

	startTime := time.Now()

	var expirerStarted int
	var expirerRunning int

	// i is uint16, because ICMP sequence number is only 16 bits
	for i, keepLooping := Sequence(0), true; i < packets && keepLooping; i++ {

		loopStartTime := time.Now()
		fakeDrop := FakeDrop(dropProb)

		if ie.Pingers.DebugLevel > 100 {
			ie.Log.Info("-------------------------------------------------")
			ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t packets:%d \t proto:%d \t keepLooping:%t \t fakeDrop:%t \t fakeSucces:%t", IP.String(), i, int(packets), proto, keepLooping, fakeDrop, fakeSuccess))
		}

		var addr *net.UDPAddr
		var wb []byte
		var merr error
		if !fakeSuccess || fakeDrop {
			msg := buildICMPMessage(id, i, proto)
			addr = &net.UDPAddr{IP: IP.IPAddr().IP, Port: 0}
			wb, merr = msg.Marshal(nil)
			if merr != nil {
				log.Fatal(fmt.Sprintf("Pinger [%s] msg.Marshal(nil):%v", IP.String(), merr))
			}
		}

		if ie.Pingers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Pinger [%s] Trying to acquire lock, to PushBack(*ps)", IP.String()))
		}
		ie.Lock() // <---------------------- LOCK!!

		// time.Now() AFTER we have acquired the lock, because it could take time to acquire
		send := time.Now()
		expiry := send.Add(ie.Timeout)
		ps := &Pings{
			NetaddrIP: IP,
			Seq:       i,
			Send:      send,
			Expiry:    expiry,
			FakeDrop:  fakeDrop,
		}

		if i == Sequence(0) {
			_, exists := ie.Pingers.Pings[IP]
			if !exists {
				ie.Pingers.Pings[IP] = make(map[Sequence]*list.Element)
			}
		}
		ie.Pingers.Pings[IP][i] = ie.Pingers.ExpiresDLL.PushBack(*ps)

		if ie.CheckExpirerIsRunning() {
			expirerStarted++
		} else {
			expirerRunning++
		}
		// Please note we must unlock AFTER we ie.CheckExpirerIsRunning()

		ie.Unlock() // <-------------------- UNLOCK!!

		if ie.Pingers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Pinger [%s] ie.Unlock()", IP.String()))
		}

		if ie.Pingers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Pinger [%s] \t expirerStarted:%d \t expirerRunning:%d ", IP.String(), expirerStarted, expirerRunning))
		}

		if fakeSuccess {
			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] \t fakeSuccess, so don't send, and the expirer will fake the success", IP.String()))
			}
		} else {
			if fakeDrop {
				if ie.Pingers.DebugLevel > 100 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \t fakeDrop, so we just don't send it, and expirer will think it's dropped", IP.String()))
				}
			} else {
				if ie.Pingers.DebugLevel > 100 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \t WriteTo len(wb):%d", IP.String(), len(wb)))
				}

				WriteTo(wb, addr, socket, ie.Pingers.DebugLevel, ie.Log)
			}
		}

		if ie.Pingers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Pinger [%s] \t select", IP.String()))
		}
		select {
		case ps := <-successCh:
			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] <-ie.SuccessChs[IP]\ti:%d", IP.String(), i))
			}
			val := ps.RTT

			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] results.RTTs:%s", IP.String(), results.RTTs))
			}
			results.RTTs[i] = val
			results.Sum += val
			//{--------------------
			// Welford's starts
			if results.Successes == 0 {
				results.Min = val
				results.Max = val
			} else {
				if val < results.Min {
					results.Min = val
				}
				if val > results.Max {
					results.Max = val
				}
			}
			results.Successes++
			oldMean := results.Mean
			results.Mean += time.Duration(float64(val-oldMean) / float64(results.Successes))
			//s.s += (val - old_mean) * (val - s.mean)
			// I'm doing something incorrectly with the variance conversions
			results.Variance += time.Duration(((val.Seconds() - oldMean.Seconds()) * (val.Seconds() - results.Mean.Seconds()))) * time.Second
			if ie.Pingers.DebugLevel > 1000 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] \ti:%d \tval:%s \tMean:%s \toldMean:%s", IP.String(), i, val.String(), results.Mean.String(), oldMean.String()))
			}
			// Welford's ends
			//}--------------------

			if ie.Pingers.DebugLevel > 100 {
				if i%(packets/PingerFractionModulo) == 0 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \ti:%d \t Seq:%d \t /%d \t RTT:%s", IP.String(), i, ps.Seq, packets, ps.RTT.String()))
				}
			}
			if ps.Seq != Sequence(i) {
				results.OutOfOrder++
				if ie.Pingers.DebugLevel > 10 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \ti:%d \t Seq:%d \t Out of order:%d", IP.String(), i, ps.Seq, results.OutOfOrder))
				}
			}
		case pe := <-expiredCh:
			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] <-ie.ExpiredChs[IP]\ti:%d", IP.String(), i))
			}
			results.Failures++
			if ie.Pingers.DebugLevel > 10 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t Seq:%d \t Expired/Timed-out after:%s", IP.String(), i, pe.Seq, ie.Timeout.String()))
			}
		case <-DoneCh:
			keepLooping = false
			if ie.Pingers.DebugLevel > 10 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] i:%d\t <-ie.Pingers.DonesChs[IP]", IP.String(), i))
			}
		case <-pingersAllDone:
			keepLooping = false
			if ie.Pingers.DebugLevel > 10 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] i:%d\t <-ie.Pingers.DoneCh", IP.String(), i))
			}
			// NO DEFAULT - This is a BLOCKING select
			//default:
		}
		if i >= packets {
			keepLooping = false
			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t i >= packets keepLooping:%t", IP.String(), i, keepLooping))
			}
		} else {
			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t i < packets keepLooping:%t", IP.String(), i, keepLooping))
			}
		}

		if keepLooping {
			loopEndTime := time.Now()
			loopDuration := loopEndTime.Sub(loopStartTime)
			sleepDuration := interval - loopDuration
			if ie.Pingers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t  \t loopDuration:%s\t sleepDuration:%s", IP.String(), i, loopDuration.String(), sleepDuration.String()))
			}
			select {
			case <-time.After(sleepDuration):
				if ie.Pingers.DebugLevel > 100 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t wakes up", IP.String(), i))
				}
			case <-DoneCh:
				keepLooping = false
				if ie.Pingers.DebugLevel > 10 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t <-ie.Pingers.DonesChs[IP]", IP.String(), i))
				}
			case <-pingersAllDone:
				keepLooping = false
				if ie.Pingers.DebugLevel > 10 {
					ie.Log.Info(fmt.Sprintf("Pinger [%s] \t i:%d \t <-ie.Pingers.DoneCh", IP.String(), i))
				}
				// NO DEFAULT - This is a BLOCKING select
				//default:
			}
		}
	}

	// Bigint for square root of int64?
	// https://golang.org/pkg/math/big/#pkg-overview
	//bigInt := &big.Int{}

	endTime := time.Now()
	results.PingerDuration = endTime.Sub(startTime)

	results.Count = results.Successes + results.Failures

	if ie.Pingers.DebugLevel > 100 {
		// The vast majority of the time these should match, but if we do kill the Pingers early, like on shutdown, then they may not match
		if results.Count != int(packets) {
			ie.Log.Info(fmt.Sprintf("Pinger [%s] results.Count:%d != int(packets):%d", IP.String(), results.Count, int(packets)))
		}
	}

	if ie.Pingers.DebugLevel > 10 {
		ie.Log.Info(fmt.Sprintf("Pinger [%s] \tsuccesses:%d \tfailures:%d \tooo:%d \tcount:%d", IP.String(), results.Successes, results.Failures, results.OutOfOrder, results.Count))
		ie.Log.Info(fmt.Sprintf("Pinger [%s] \tmin:%s \tmax:%s \tmean:%s \tvariance:%s \tsum:%s \tPingerDuration:%s", IP.String(), results.Min.String(), results.Max.String(), results.Mean.String(), results.Variance.String(), results.Sum.String(), results.PingerDuration.String()))
	}

	if sortRTTs {
		sort.Slice(results.RTTs, func(i, j int) bool { return results.RTTs[i] < results.RTTs[j] })
	}

	if EdebugLevel > 10 {
		ie.Log.Info(fmt.Sprintf("Pinger [%s] \t Acquiring ie.Lock() to delete", IP.String()))
	}
	ie.Lock()
	delete(ie.Pingers.Pings, IP)
	delete(ie.Pingers.SuccessChs, IP)
	delete(ie.Pingers.ExpiredChs, IP)
	delete(ie.Pingers.DonesChs, IP)
	ie.Unlock()
	ie.Log.Info(fmt.Sprintf("Pinger [%s] Map keys deleted, and lock released, returning", IP.String()))

	return results
}

// FakeDrop is a simple function to return true based on a probability
// Looking at this issue, I'm not sure if this is perfect, but should be ok
// https://github.com/golang/go/issues/12290
func FakeDrop(dropProb float64) (drop bool) {

	if dropProb > 0 {
		if rand.Float64() >= (1 - dropProb) {
			drop = true
		}
	}
	return
}

// WriteTo performs the socket write, and does error handling
func WriteTo(wb []byte, addr *net.UDPAddr, socket *icmp.PacketConn, debugLevel int, logger hclog.Logger) {

	var bw int
	var we error
	bw, we = (socket).WriteTo(wb, addr) // ----------------------------<< WriteTo ( Sends packet to the kernel )
	if we != nil {
		if debugLevel > 100 {
			logger.Error(fmt.Sprintf("Pinger [%s] \t Writer bytes error:%s", addr.IP.String(), we))
		}
		if neterr, ok := we.(*net.OpError); ok {
			if neterr.Err == syscall.ENOBUFS {
				log.Fatal(neterr)
			}
		}
	}
	if bw != len(wb) {
		log.Fatal("Pinger WriteTo error. Bytes sent does not match packet length.")
	}
	if debugLevel > 100 {
		logger.Info(fmt.Sprintf("Pinger [%s] WriteTo bytes written:%d \t len(wb):%d \t to:[%s]", addr.IP.String(), bw, len(wb), (socket).LocalAddr()))
	}
}

// buildICMPMessage builds the icmp.Echo message body and the icmp.Message
func buildICMPMessage(id int, seq Sequence, proto Protocol) (msg *icmp.Message) {

	body := &icmp.Echo{
		ID:  id,
		Seq: int(seq),
	}

	if proto == Protocol(4) {
		msg = &icmp.Message{
			Type: ipv4.ICMPTypeEcho,
			Code: 0,
			Body: body,
		}
	} else {
		msg = &icmp.Message{
			Type: ipv6.ICMPTypeEchoRequest,
			Code: 0,
			Body: body,
		}
	}

	return msg
}
