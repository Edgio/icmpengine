// Copyright 2021 Edgio Inc

package icmpengine

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"inet.af/netaddr"
)

const (
	//ReceiveBufferMax = 1500 // max packet receive size
	ReceiveBufferMax = 200 // max packet receive size

	// Timeouts In A Row (tiar) to slowly back off the socket deadline timeout
	tiarLow    = 5
	tiarMedium = 10
	tiarHigh   = 20

	multiLow    = 2
	multiMedium = 10
	multiHigh   = 20

	RdebugLevel = 111
)

// sync.Pool in theory reduces garbage collection
var bufPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, ReceiveBufferMax)
		return &b
	},
}

// timeoutsInARowCalculator (Timeouts In A Row) is just a simple function to return
// the multiplier amount based on a simple table.
// This is just a wrapper using constants around the tiarCalculator
func timeoutsInARowCalculator(timeoutsInARow int) (multiplier float64) {
	return tiarCalculator(timeoutsInARow, tiarLow, tiarMedium, tiarHigh, multiLow, multiMedium, multiHigh)
}

// tiarCalculator (Timeouts In A Row) is just a simple function to return
// the multiplier amount based on a simple table.  Probably a tree structure
// could be used, or a y = mX+c style function, but this will do the job
// tiarCalculator is seperated to allow for testing
func tiarCalculator(tiar int, low int, medium int, high int, mLow float64, mMedium float64, mHigh float64) (multiplier float64) {
	multiplier = 1
	if tiar >= high {
		multiplier = mHigh
	} else {
		if tiar >= medium {
			multiplier = mMedium
		} else {
			if tiar >= low {
				multiplier = mLow
			}
		}
	}
	return multiplier
}

// Receiver receives ICMP messages, calculates the round-trip-time(RTT) and then send the response to the requesting Pinger
// Receiver is also responsible for tracking the timeouts, using the double-linked-list and map
// ie.ReadDeadline is used to not just block forever on the read call, so we can check the Done channel has been called
// When choosing the ReadDeadline, it's just changing how quickly the Receiver might detect the Done signal
//
// Because ReadFrom syscall is blocking, a SetReadDeadline is used to allow ReadFrom to finish,
// this is mostly to allow checking for the done signal, and therefore allow closing down the Receivers
// gracefully.
// There is [Timeouts In A Row] code that increases these timeouts gradually, to decrease the ReadFrom thrashing
//
func (ie *ICMPEngine) Receiver(proto Protocol, index int, allDone <-chan struct{}, done <-chan struct{}) {

	if ie.Sockets.DebugLevel > 100 {
		ie.Log.Info("Receiver \t proto:%d \t index:%d acquiring ie.RLock()")
	}
	ie.RLock()
	fakeSuccess := ie.Expirers.FakeSuccess
	ie.RUnlock()
	if ie.Sockets.DebugLevel > 100 {
		ie.Log.Info("Receiver \t proto:%d \t index:%d released ie.RLock()")
	}

	// Don't start the receivers if we're faking success
	if fakeSuccess {
		// return
		log.Fatal(fmt.Sprintf("Receiver fakeSuccess:%t nothing should try to start the receivers", fakeSuccess))
	}

	if ie.Receivers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Receiver\t proto:%d \t index:%d, start \t Receiver c:%s", proto, index, (ie.Sockets.Sockets[proto]).LocalAddr()))
	}

	defer ie.Receivers.WG.Done()

	for i, keepLooping, timeouts, timeoutsInARow := 0, true, 0, 0; keepLooping; i++ {

		//buffer := make([]byte, ReceiveBufferMax)
		buffer := bufPool.Get().(*[]byte)

		// We increase the timeouts when there have been a lot of timeouts in a row, to reduce thrashing on the syscall
		var readDealLine time.Duration = time.Duration(float64(ie.ReadDeadline) * timeoutsInARowCalculator(timeoutsInARow))

		(ie.Sockets.Sockets[proto]).SetReadDeadline(time.Now().Add(readDealLine))
		if ie.Receivers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Receiver\t proto:%d \t index:%d, ReadFrom start with timeout, i:%d \t readDealLine:%s \t keepLooping:%t \tTimeouts:%d \t timeoutsInARow:%d", proto, index, i, readDealLine.String(), keepLooping, timeouts, timeoutsInARow))
		}

		n, peer, err := (ie.Sockets.Sockets[proto]).ReadFrom(*buffer) // <------------------------- ReadFrom (blocking until timeout)
		receiveTime := time.Now()
		if err != nil {
			if err.(net.Error).Timeout() {
				timeouts++
				timeoutsInARow++
				if ie.Receivers.DebugLevel > 100 {
					ie.Log.Info(fmt.Sprintf("Receiver\t proto:%d \t index:%d, ReadFrom Timeouts:%d \t timeoutsInARow:%d", proto, index, timeouts, timeoutsInARow))
				}
				// Do NOT continue here, because we need to check the done channel below
			} else {
				if ie.Receivers.DebugLevel > 100 {
					ie.Log.Info(fmt.Sprintf("Receiver\t proto:%d \t index:%d, ReadFrom actual error", proto, index))
				}
				log.Fatal(fmt.Sprintf("Receiver\t proto:%d \t index:%d \t err:%v", proto, index, err))
			}
		} else {
			timeoutsInARow = 0
		}

		if n > 0 {

			if ie.Receivers.DebugLevel > 1000 {
				ie.Log.Info(fmt.Sprintf("Receiver\t proto:%d \t index:%d, receiveTime:%s\t n:%d\t peer:%s", proto, index, receiveTime, n, peer))
			}

			echoReply, err := ParseICMPEchoReply(*buffer)

			if err != nil {
				ie.Log.Info(fmt.Sprintf("Receiver\t proto:%d \t index:%d, ParseMessage error:%s", proto, index, err))
			} else {

				host, _, err := net.SplitHostPort(peer.String())
				if err != nil {
					if ie.Receivers.DebugLevel > 100 {
						ie.Log.Info(fmt.Sprintf("Receiver \t proto:%d \t index:%d, SplitHostPort error::%s", proto, index, err))
					}
				}
				ip := netaddr.MustParseIP(host)
				s := Sequence(echoReply.Seq)

				ie.RLock() // <------------------ READ LOCK!!
				el, exists := ie.Pingers.Pings[ip][s]
				ie.RUnlock() // <---------------- READ UNLOCK!!

				if !exists {
					if ie.Receivers.DebugLevel > 10 {
						ie.Log.Info(fmt.Sprintf("Receiver [%s] \t proto:%d \t index:%d, Unknown ICMP reply message.  Where on earth did this come from??!!", ip.String(), proto, index))
					}
				} else {
					rttDuration := receiveTime.Sub(el.Value.(Pings).Send)
					if ie.Receivers.DebugLevel > 100 {
						ie.Log.Info(fmt.Sprintf("Receiver [%s] \t Exists \t proto:%d \t index:%d, m.Seq:%d\t rttDuration:%s", ip.String(), proto, index, echoReply.Seq, rttDuration.String()))
					}

					ps := &PingSuccess{
						Seq:      s,
						Send:     el.Value.(Pings).Send,
						Received: receiveTime,
						RTT:      rttDuration,
					}
					ie.Lock() // <--------------- LOCK!!
					ie.Pingers.SuccessChs[ip] <- *ps
					delete(ie.Pingers.Pings[ip], s)
					ie.Pingers.ExpiresDLL.Remove(el)
					ie.Unlock() // <------------- UNLOCK!!
					if ie.Receivers.DebugLevel > 100 {
						ie.Log.Info(fmt.Sprintf("Receiver [%s] \t proto:%d \t index:%d, ie.SuccessChs[ip] <- *ps, delete, remove from  ExpiresDLL", ip.String(), proto, index))
					}
				}
			}
		}
		bufPool.Put(buffer)

		select {
		case <-allDone:
			if ie.Receivers.DebugLevel > 10 {
				ie.Log.Info(fmt.Sprintf("Receiver \t proto:%d \t index:%d, <-allDone", proto, index))
			}
			keepLooping = false
		case <-done:
			if ie.Receivers.DebugLevel > 10 {
				ie.Log.Info(fmt.Sprintf("Receiver \t proto:%d \t index:%d, <-done", proto, index))
			}
			keepLooping = false
		default:
			// non-block
		}

		if ie.Receivers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Receiver \t proto:%d \t index:%d, end of for loop, i:%d", proto, index, i))
		}
	}
	if ie.Receivers.DebugLevel > 10 {
		ie.Log.Info(fmt.Sprintf("Receiver \t proto:%d \t index:%d, done", proto, index))
	}
}
