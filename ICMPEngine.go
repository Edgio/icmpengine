package icmpengine

// This file contains the main ICMPEngine data structures
// and New()

// IPPROTO_ICMP sockets which are NonPrivilegedPing
// https://lwn.net/Articles/422330/

// [Sun May 16 04:31:11] root@cache17.tla:~# sysctl net.ipv4.ping_group_range
// net.ipv4.ping_group_range = 1	0
// [Sat Jun 12 16:58:53] root@bbr005.ebx:~# sysctl net.ipv4.ping_group_range
// net.ipv4.ping_group_range = 1	0

// Default
// sudo sysctl -w net.ipv4.ping_group_range="1 0"
// Set to run
// sudo sysctl -w net.ipv4.ping_group_range="0 2147483647"

import (
	"container/list"
	"fmt"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"golang.org/x/net/icmp"
	"inet.af/netaddr"
)

const (
	//"golang.org/x/net/internal/iana"
	ProtocolICMP     = 1  // Internet Control Message
	ProtocolIPv6ICMP = 58 // ICMP for IPv6

	Receivers4Cst         = 2
	Receivers6Cst         = 2
	OpenSocketsRetriesCst = 2

	SplayReceiversCst = true

	IEdebugLevel = 111
)

// ICMPEngine holds the object state
// Most of this data is for tracking ICMP echo requests sent, and their expiry times
// The double linked-list (DLL) allows tracking the next Expiry time, while allowing
// entries to be removed efficently when a ping is recieved.
// Leveraging https://golang.org/pkg/container/list/
// Need to move to https://golang.org/pkg/container/heap/
type ICMPEngine struct {
	Log hclog.Logger
	sync.RWMutex
	Timeout      time.Duration
	ReadDeadline time.Duration
	Protocols    []Protocol
	PID          int
	EID          int
	DoneCh       chan struct{}
	Sockets      SocketsT
	Receivers    ReceiversT
	Expirers     ExpirersT
	Pingers      PingersT
	DebugLevel   int
}

type SocketsT struct {
	Open       bool
	Opens      map[Protocol]bool
	Networks   map[Protocol]string
	Addresses  map[Protocol]string
	Sockets    map[Protocol]*icmp.PacketConn
	DebugLevel int
}

type ReceiversT struct {
	WG         sync.WaitGroup
	DoneCh     chan struct{}
	DoneChs    map[Protocol]chan struct{}
	Counts     map[Protocol]int
	Runnings   map[Protocol]bool
	Running    bool
	DebugLevel int
}

type ExpirersT struct {
	WG          sync.WaitGroup
	DoneCh      chan struct{}
	DonesChs    map[Protocol]chan struct{}
	Runnings    map[Protocol]bool
	Running     bool
	DebugLevel  int
	FakeSuccess bool
}

type PingersT struct {
	WG         sync.WaitGroup
	DoneCh     chan struct{}
	Pings      map[netaddr.IP]map[Sequence]*list.Element
	ExpiresDLL *list.List
	SuccessChs map[netaddr.IP]chan PingSuccess
	ExpiredChs map[netaddr.IP]chan PingExpired
	DonesChs   map[netaddr.IP]chan struct{}
	DebugLevel int
}

type Sequence uint16
type WorkerType rune
type Protocol uint8
type Pings struct {
	NetaddrIP netaddr.IP
	Seq       Sequence
	Send      time.Time
	Expiry    time.Time
	FakeDrop  bool
}

// PingSuccess is passed from the Receivers to the Pingers
type PingSuccess struct {
	Seq      Sequence
	Send     time.Time
	Received time.Time
	RTT      time.Duration
}

// PingExpired is passed from the Expirer to the Pingers
// This only happens when there is a timeout (obviously)
type PingExpired struct {
	Seq  Sequence
	Send time.Time
}

type DebugLevelsT struct {
	IE int
	S  int
	R  int
	E  int
	P  int
}

// GetDebugLevels is a little helper function to return DebugLevelsT
// filled in with the same debug level for each component
func GetDebugLevels(debuglevel int) (debugLevels DebugLevelsT) {
	debugLevels = DebugLevelsT{
		IE: debuglevel,
		S:  debuglevel,
		R:  debuglevel,
		E:  debuglevel,
		P:  debuglevel,
	}
	return debugLevels
}

// New creates ICMPEngine with default Receivers Per Protocol
func New(l hclog.Logger, done chan struct{}, to time.Duration, rd time.Duration, start bool) (icmpEngine *ICMPEngine) {

	var debugLevels = DebugLevelsT{
		IE: IEdebugLevel,
		S:  SdebugLevel,
		R:  RdebugLevel,
		E:  EdebugLevel,
		P:  PdebugLevel,
	}

	return NewFullConfig(l, done, to, rd, start, Receivers4Cst, Receivers6Cst, debugLevels, false)
}

// NewFullConfig creates ICMPEngine with the full set of configuration options
// Please note could icmpEngine.Start()
// It is recommended NOT to actually start until you really need ICMPengine listening for incoming packets
// e.g. You can defer opening the sockets, and starting the receivers until you actually need them
func NewFullConfig(logger hclog.Logger, done chan struct{}, timeout time.Duration, deadline time.Duration, start bool, receivers4 int, receivers6 int, debugLevels DebugLevelsT, fakeSuccess bool) (icmpEngine *ICMPEngine) {

	rand.Seed(time.Now().UnixNano())

	// Make all the maps here, but create all the channels as part of Start() in StartChannels()
	icmpEngine = &ICMPEngine{
		Log:          logger,
		Timeout:      timeout,
		ReadDeadline: deadline,
		Protocols:    []Protocol{Protocol(4), Protocol(6)},
		PID:          os.Getpid() & 0xffff,
		EID:          os.Geteuid(),
		DoneCh:       done,
		DebugLevel:   debugLevels.IE,
		Sockets: SocketsT{
			Networks:   make(map[Protocol]string),
			Addresses:  make(map[Protocol]string),
			Sockets:    make(map[Protocol]*icmp.PacketConn),
			Opens:      make(map[Protocol]bool),
			DebugLevel: debugLevels.S,
		},
		Receivers: ReceiversT{
			DoneCh:     make(chan struct{}, 2),
			DoneChs:    make(map[Protocol]chan struct{}),
			Counts:     make(map[Protocol]int),
			Runnings:   make(map[Protocol]bool),
			DebugLevel: debugLevels.R,
		},
		Expirers: ExpirersT{
			DoneCh:      make(chan struct{}, 2),
			DonesChs:    make(map[Protocol]chan struct{}),
			Runnings:    make(map[Protocol]bool),
			DebugLevel:  debugLevels.E,
			FakeSuccess: fakeSuccess,
		},
		Pingers: PingersT{
			Pings:      make(map[netaddr.IP]map[Sequence]*list.Element),
			ExpiresDLL: list.New(),
			SuccessChs: make(map[netaddr.IP]chan PingSuccess),
			ExpiredChs: make(map[netaddr.IP]chan PingExpired),
			DonesChs:   make(map[netaddr.IP]chan struct{}),
			DebugLevel: debugLevels.P,
		},
	}

	icmpEngine.Receivers.Counts[Protocol(4)] = receivers4
	icmpEngine.Receivers.Counts[Protocol(6)] = receivers6

	icmpEngine.Sockets.Networks[Protocol(4)] = "udp4"
	icmpEngine.Sockets.Networks[Protocol(6)] = "udp6"
	icmpEngine.Sockets.Addresses[Protocol(4)] = "0.0.0.0"
	icmpEngine.Sockets.Addresses[Protocol(6)] = "::"

	if start {
		icmpEngine.Start()
	}

	return icmpEngine
}

// StartReceivers starts the receivers, with default to splay the receiver start times
// This essentailly means ReadFrom() syscall will be called every ReadDeadline period of time
func (ie *ICMPEngine) StartReceivers() {
	ie.StartReceiversSplay(true)
}

// StartReceiversSplay starts the receivers, with some sanity checking
func (ie *ICMPEngine) StartReceiversSplay(splay bool) {

	if ie.DebugLevel > 10 {
		ie.Log.Info(fmt.Sprintf("StartReceiversSplay splay:%t", splay))
	}

	ie.RLock()
	if ie.Receivers.Running {
		if ie.DebugLevel > 10 {
			ie.Log.Info("StartReceiversSplay ie.Receivers.Running")
		}
		ie.RUnlock()
		// return
		log.Fatal("StartReceiversSplay ie.Receivers.Running")
	}
	ie.RUnlock()

	if ie.DebugLevel > 10 {
		ie.Log.Info("StartReceiversSplay acquiring lock")
	}

	ie.Lock()
	defer ie.Unlock()

	if ie.DebugLevel > 10 {
		ie.Log.Info("StartReceiversSplay ie.Lock() acquired, defer defer ie.Unlock()")
	}

	var receivers int
	for _, p := range ie.Protocols {
		done := make(chan struct{}, 2)
		ie.Receivers.DoneChs[p] = done
		if ie.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("StartReceiversSplay ie.Receivers.DoneChs[%d] = make(chan struct{},2)", p))
		}
		for r := 0; r < ie.Receivers.Counts[p]; r++ {
			ie.Receivers.WG.Add(1)
			go ie.Receiver(p, r, ie.Receivers.DoneCh, done)
			receivers++
			if ie.DebugLevel > 100 {
				ie.Log.Info(fmt.Sprintf("StartReceiversSplay go ie.Receiver(p, r) started protocol:%d \t r:%d", p, r))
			}
			if splay {
				sleepDuration := time.Duration(float64(ie.ReadDeadline) / float64(ie.Receivers.Counts[p]))
				if ie.DebugLevel > 100 {
					ie.Log.Info(fmt.Sprintf("StartReceiversSplay Receivers start delay:%s", sleepDuration.String()))
				}
				select {
				case <-time.After(sleepDuration):
					if ie.DebugLevel > 100 {
						ie.Log.Info("StartReceiversSplay wakes up")
					}
				case <-ie.Receivers.DoneCh:
					if ie.DebugLevel > 100 {
						ie.Log.Info("StartReceiversSplay <-ie.Receivers.DoneCh")
					}
					return
				case <-ie.DoneCh:
					if ie.DebugLevel > 100 {
						ie.Log.Info("StartReceiversSplay <-ie.DoneCh")
					}
					return
					// NO DEFAULT - This is a BLOCKING select
					//default:
				}
			}
		}
	}

	if ie.DebugLevel > 10 {
		ie.Log.Info(fmt.Sprintf("StartReceiversSplay Started \t receivers:%d (and defer mutex unlock)", receivers))
	}
}

// OpenDoneChannels opens the main done channel for each worker type
func (ie *ICMPEngine) OpenDoneChannels(fakeSuccess bool) {

	if !fakeSuccess {
		ie.Receivers.DoneCh = make(chan struct{}, 2)
	}
	ie.Expirers.DoneCh = make(chan struct{}, 2)
	ie.Pingers.DoneCh = make(chan struct{}, 2)

	if ie.DebugLevel > 10 {
		ie.Log.Info("OpenDoneChannels() complete")
	}
}

// Start OpenSockets and starts the Receivers, with default to splay the receiver start times
// This isn't done on New or NewRPP, to avoid opening the sockets and
// having the Receivers busy making recieve syscalls until ICMPEngine really needs
// to be running.  e.g. ICMPEngine "object" can be created once, but
// not actually running much until Start() is called
// This is possibly an premature optimization.
func (ie *ICMPEngine) Start() {
	ie.StartSplay(SplayReceiversCst)
}

// StartSplay OpenSockets and starts the Receivers
func (ie *ICMPEngine) StartSplay(splay bool) {

	if ie.DebugLevel > 10 {
		ie.Log.Info("ICMPEngine Start")
	}

	ie.RLock()
	fakeSuccess := ie.Expirers.FakeSuccess
	ie.RUnlock()

	ie.OpenDoneChannels(fakeSuccess)

	// Don't open sockets if we're faking success
	if fakeSuccess {
		if ie.DebugLevel > 10 {
			ie.Log.Info(fmt.Sprintf("ICMPEngine StartSplay not opening sockets or starting receivers fakeSuccess:%t", fakeSuccess))
		}
		return
	}

	ie.OpenSockets()

	ie.StartReceiversSplay(splay)

	if ie.DebugLevel > 10 {
		ie.Log.Info("ICMPEngine Started")
	}
}

func (ie *ICMPEngine) Run(wg *sync.WaitGroup) {

	ie.RLock()
	fakeSuccess := ie.Expirers.FakeSuccess
	ie.RUnlock()

	if ie.DebugLevel > 100 {
		ie.Log.Info(fmt.Sprintf("Run() fakeSuccess:%t", fakeSuccess))
	}

	defer wg.Done()

	if ie.DebugLevel > 100 {
		ie.Log.Info("Run() Waiting for done")
	}

	<-ie.DoneCh // Block waiting for a signal to shutdown ----------------------------- BLOCK!!
	if ie.DebugLevel > 10 {
		ie.Log.Info("Run() received done, calling Stop()")
	}
	ie.Stop(fakeSuccess)

	if ie.DebugLevel > 10 {
		ie.Log.Info("Run() done")
	}
}

// Stop gracefully stops the workers
func (ie *ICMPEngine) Stop(fakeSuccess bool) {

	if ie.DebugLevel > 10 {
		ie.Log.Info("Stop()")
	}

	if ie.DebugLevel > 10 {
		ie.Log.Info("close(ie.Pingers.DoneCh) and ie.Pingers.WG.Wait()")
	}

	close(ie.Pingers.DoneCh)
	ie.Pingers.WG.Wait()

	if ie.DebugLevel > 10 {
		ie.Log.Info("Stop() ie.Pingers.WG.Wait() complete")
	}

	if ie.DebugLevel > 10 {
		ie.Log.Info("close(ie.Expirers.DoneCh) and ie.Expirers.WG.Wait()")
	}
	close(ie.Expirers.DoneCh)
	ie.Expirers.WG.Wait()

	if ie.DebugLevel > 10 {
		ie.Log.Info("Stop() ie.Expirers.WG.Wait() complete")
	}

	if !fakeSuccess {
		if ie.DebugLevel > 10 {
			ie.Log.Info("close(ie.Receivers.DoneCh) and ie.Receivers.WG.Wait()")
		}
		close(ie.Receivers.DoneCh)
		// probably don't need this, but it doesn't hurt
		for _, p := range ie.Protocols {
			close(ie.Receivers.DoneChs[p])
		}
		ie.Receivers.WG.Wait()

		if ie.DebugLevel > 10 {
			ie.Log.Info("Stop() ie.Receivers.WG.Wait() complete.  Calling ie.CloseSockets()")
		}

		ie.CloseSockets()
	} else {
		if ie.DebugLevel > 100 {
			ie.Log.Info(fmt.Sprintf("Stop() fakeSuccess:%t not stopping receivers, and not closing sockets", fakeSuccess))
		}
	}

	if ie.DebugLevel > 10 {
		ie.Log.Info("Stop() complete")
	}

	//Run() has defer wg.Done()
}
