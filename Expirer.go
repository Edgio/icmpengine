// Copyright 2021 Edgecast Inc

package icmpengine

// Expirer holds a sinlge sleep timer

// Socketsider using heap, rather than ordered DLL, which would allow for different timeout values
// https://golang.org/pkg/container/heap/

// Notes
// Currently have a single expirer, but could easily have one per protocol which would provide more parallelism
// The other suggestion is to make a single expiry for a batch of packets, rather than one per packet.
// Both these ideas could help if performance becomes an issue, which currnetly it is not.

import (
	"container/list"
	"fmt"
	"time"
)

const (
	EdebugLevel = 111
)

// CheckExpirerIsRunning checks Expirers is running, and starts it if required
// returns if Expirers was started
// CheckExpirerIsRunning assumes the LOCK is already held by Pinger
func (ie *ICMPEngine) CheckExpirerIsRunning() (started bool) {

	if ie.Expirers.DebugLevel > 100 {
		ie.Log.Info("CheckExpirerIsRunning() start")
	}
	if ie.Expirers.Running {
		if ie.Expirers.DebugLevel > 100 {
			ie.Log.Info("CheckExpirerIsRunning ie.Expirers.Running")
		}
		started = false
	} else {
		ie.Expirers.WG.Add(1)
		go ie.ExpirerConfig(ie.Expirers.FakeSuccess)
		ie.Expirers.Running = true

		if ie.Expirers.DebugLevel > 100 {
			ie.Log.Info("CheckExpirerIsRunning started")
		}
		started = true
	}
	return started
}

// Expirer tracks the ICMP echo timeouts
// The idea is to just have the single and nearest timer running at any single moment
// The "Config" implies that we can configure the FakeSuccess, which is used for testing
func (ie *ICMPEngine) ExpirerConfig(FakeSuccess bool) {

	if ie.Expirers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Expirer start \t FakeSuccess:%t", FakeSuccess))
	}

	defer ie.Expirers.WG.Done()

	ie.RLock()
	done := ie.Expirers.DoneCh
	ie.RUnlock()

	for i, keepLooping := 0, true; keepLooping; i++ {

		var el *list.Element
		var exists bool
		var len int
		var SoonestPing Pings
		var sleepDuration time.Duration

		if ie.Expirers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Expirer \t i:%d", i))
		}
		select {
		case <-done:
			if ie.Expirers.DebugLevel > 10 {
				ie.Log.Info("Expirer received done")
			}
			keepLooping = false
			continue
		default:
			// non-block
		}

		if ie.Expirers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Expirer trying to acquire ie.Lock() to check len\t i:%d", i))
		}
		ie.Lock() // <-------------------------- LOCK!!
		len = ie.Pingers.ExpiresDLL.Len()
		if len == 0 {
			ie.Expirers.Running = false
			ie.Unlock() // <-------------------- UNLOCK!!
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Expirer ie.Unlock(). No more elements in expires list, len:%d.  Returning", len))
			}
			keepLooping = false
			return
		}

		el = ie.Pingers.ExpiresDLL.Front()
		SoonestPing = copyPing(ie.Pingers.ExpiresDLL.Front())

		if FakeSuccess && !SoonestPing.FakeDrop {
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info("Expirer FakeSuccess doing delete")
			}
			successCh := ie.Pingers.SuccessChs[SoonestPing.NetaddrIP]
			ie.Pingers.ExpiresDLL.Remove(el)
			delete(ie.Pingers.Pings[SoonestPing.NetaddrIP], SoonestPing.Seq)
			ie.Unlock() // <-------------------- UNLOCK!!
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info("Expirer FakeSuccess ie.Unlock()")
			}
			fakeReceivedTime := time.Now()
			rttDuration := fakeReceivedTime.Sub(SoonestPing.Send)
			successCh <- PingSuccess{
				Seq:      SoonestPing.Seq,
				Send:     SoonestPing.Send,
				Received: fakeReceivedTime,
				RTT:      rttDuration,
			}
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Expirer \t i:%d Sent <- PingSuccess FakeSuccess", i))
			}
			continue
		}
		ie.Unlock() // <------------------------ UNLOCK!!

		sleepDuration = time.Until(SoonestPing.Expiry)

		if ie.Expirers.DebugLevel > 1000 {
			ie.Log.Info(fmt.Sprintf("Expirer \t i:%d going to sleep duration:%s", i, sleepDuration.String()))
		}

		select {
		case <-time.After(sleepDuration):
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Expirer wakes up after duration:%s", sleepDuration.String()))
			}
		case <-done:
			if ie.Expirers.DebugLevel > 10 {
				ie.Log.Info("Expirer was sleeping, but received done")
			}
			keepLooping = false
			// NO DEFAULT - This is BLOCKING
			//default:
		}

		if ie.Expirers.DebugLevel > 100 {
			ie.Log.Info("Expirer trying to acquire ie.RLock() to check exists")
		}
		ie.RLock() // <-------------------------- READ LOCK!!
		el, exists = ie.Pingers.Pings[SoonestPing.NetaddrIP][SoonestPing.Seq]
		ie.RUnlock() // <------------------------ READ UNLOCK!!
		if ie.Expirers.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Expirer ie.RUnlock(), exists:%t", exists))
		}

		// If the key still exists, then the Receiver did NOT get a return packet, so the timeout has expired
		if exists {
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Expirer found expired \t IP:%s \t Seq:%d deleting", SoonestPing.NetaddrIP.String(), SoonestPing.Seq))
			}

			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info("Expirer exists - trying to acquire ie.Lock()")
			}

			ie.Lock() // <----------------------- LOCK!!
			delete(ie.Pingers.Pings[SoonestPing.NetaddrIP], SoonestPing.Seq)
			ie.Pingers.ExpiresDLL.Remove(el)
			expiredCh := ie.Pingers.ExpiredChs[SoonestPing.NetaddrIP]
			ie.Unlock() // <--------------------- UNLOCK!!

			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info("Expirer exists - ie.Unlock()")
			}

			expiredCh <- PingExpired{
				Seq:  SoonestPing.Seq,
				Send: SoonestPing.Send,
			}
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("Expirer \t i:%d Sent <- PingExpired", i))
			}
		} else {
			if ie.Expirers.DebugLevel > 100 {
				ie.Log.Info("Expirer expiry no longer exists, so we must have received a response.  Excellent.")
			}
		}
	}

	if ie.Expirers.DebugLevel > 100 {
		ie.Log.Info("Expirer - trying to acquire ie.Lock() to ie.Expirers.Running = false, Defer unlock")
	}
	ie.Lock()
	defer ie.Unlock()
	len := ie.Pingers.ExpiresDLL.Len()
	ie.Expirers.Running = false

	if ie.Expirers.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Expirer len:%d ie.ExpirerRunning = false.  Expirer complete. defer ie.Unlock()", len))
	}
}

// copyPing is a small helper to copy by value the ping
// this is just to help reduce code in the main function
func copyPing(el *list.Element) (ping Pings) {
	ping.NetaddrIP = el.Value.(Pings).NetaddrIP
	ping.Seq = el.Value.(Pings).Seq
	ping.Send = el.Value.(Pings).Send
	ping.Expiry = el.Value.(Pings).Expiry
	ping.FakeDrop = el.Value.(Pings).FakeDrop
	return ping
}
