package icmpengine

// Sockets holds the OpenSockets/CloseSockets functions

// IPPROTO_ICMP sockets which are NonPrivilegedPing
// https://lwn.net/Articles/422330/

import (
	"fmt"
	"log"

	"github.com/go-cmd/cmd"
	"golang.org/x/net/icmp"
)

const (
	SdebugLevel = 111
)

// OpenSockets opens non-privleged ICMP sockets for sending echo requests/replies
// OpenSockets has retry logic, and can use HackSysctl to change the sysctl
// for the non-privleged ICMP sockets if ICMPEngine is running as root
// Hopefully ICMPEngine is not running as root, in which case, if it can't
// open the sockets, it will log fatal
func (ie *ICMPEngine) OpenSockets() {

	if ie.Sockets.DebugLevel > 10 {
		ie.Log.Info("OpenSockets() acquiring ie.RLock()")
	}
	ie.RLock()
	fakeSuccess := ie.Expirers.FakeSuccess
	open := ie.Sockets.Open
	ie.RUnlock()

	// Assertion - Don't open sockets if we're faking success
	if fakeSuccess {
		// return
		log.Fatal(fmt.Sprintf("OpenSockets fakeSuccess:%t nothing should try to open the sockets", fakeSuccess))
	}

	// Assertion - don't reopen sockets
	if open {
		// return
		log.Fatal("OpenSockets ie.Sockets.Open sockets are already open")
	}

	ie.Lock()
	defer ie.Unlock()
	if ie.Sockets.DebugLevel > 10 {
		ie.Log.Info("OpenSockets ie.Lock() acquired")
	}

	if ie.Sockets.Open {
		if ie.Sockets.DebugLevel > 10 {
			ie.Log.Info("OpenSockets ie.Sockets.Open sockets are already open wih ie.Lock()")
		}
		return
	}

	var sockets int
	for _, p := range ie.Protocols {
		for retries := 0; retries < OpenSocketsRetriesCst && !ie.Sockets.Opens[p]; retries++ {
			if ie.Sockets.Opens[p] {
				if ie.Sockets.DebugLevel > 10 {
					ie.Log.Info(fmt.Sprintf("OpenSockets ie.Sockets.Opens[%d] sockets are already open. ??!", p))

				}
				//return
				log.Fatal(fmt.Sprintf("OpenSockets ie.Sockets.Opens[%d] sockets are already open. ??!", p))
			}
			var sockErr error
			ie.Sockets.Sockets[p], sockErr = icmp.ListenPacket(ie.Sockets.Networks[p], ie.Sockets.Addresses[p])
			if sockErr != nil {

				if ie.HackSysctl() {
					continue
				}
				ie.Log.Error("Please run: sudo sysctl -w net.ipv4.ping_group_range=\"0 2147483647\"")
				log.Fatal("icmp.ListenPacket sockErr:", sockErr)
			}
			ie.Sockets.Opens[p] = true
			sockets++
			if ie.Sockets.DebugLevel > 10 {
				ie.Log.Info(fmt.Sprintf("OpenSockets() Socket Open \t protocol:%d \t retries:%d", p, retries))
			}
		}
	}

	// Assertion
	if sockets != len(ie.Protocols) {
		log.Fatal(fmt.Sprintf("OpenSockets() failed to open both IPv4 and IPv6 sockets. !! sockets:%d", sockets))
	}
	ie.Sockets.Open = true

	if ie.Sockets.DebugLevel > 10 {
		ie.Log.Info("OpenSockets() ie.SocketsOpen = true")
	}
}

// CloseSockets() closes the sockets with some assertion checks
func (ie *ICMPEngine) CloseSockets() {

	if ie.Sockets.DebugLevel > 10 {
		ie.Log.Info("CloseSockets() acquiring lock")
	}

	ie.Lock()
	defer ie.Unlock()

	if ie.Sockets.DebugLevel > 10 {
		ie.Log.Info("CloseSockets() lock acquired")
	}

	var sockets int
	for _, p := range ie.Protocols {
		(*ie.Sockets.Sockets[p]).Close()
		delete(ie.Sockets.Sockets, p)
		ie.Sockets.Opens[p] = false
		sockets++
	}
	// Assertion
	if sockets != len(ie.Protocols) {
		log.Fatal(fmt.Sprintf("Shutdown() closing failed to close both IPv4 and IPv6 sockets. !! sockets:%d", sockets))
	}
	ie.Sockets.Open = false

	if ie.Sockets.DebugLevel > 10 {
		ie.Log.Info("CloseSockets() sockets closed")
	}
}

// HackSysctl does sysctl -w net.ipv4.ping_group_range=0 2147483647
// This requires root
func (ie *ICMPEngine) HackSysctl() (success bool) {
	if ie.EID != 0 {
		return success
	}
	// No need quote the same way as you do from bash
	sysctlCmd := cmd.NewCmd(`sysctl`, `-w`, `net.ipv4.ping_group_range=0 2147483647`)
	status := <-sysctlCmd.Start()
	if ie.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("HackSysctl status:%v", status))
		for _, line := range status.Stdout {
			ie.Log.Info(fmt.Sprintf("HackSysctl line:%s", line))
		}
	}
	success = true
	return success
}
