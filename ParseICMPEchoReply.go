package icmpengine

import (
	"bytes"
	"encoding/binary"
	"errors"
)

var (
	errMessageTooShort = errors.New("message too short")
)

// ICMPEchoReply represents Echo Reply messages
// per the IPv4/rfc792 and IPv6/rfc2463 ( see extended comments below )
type ICMPEchoReply struct {
	Type       uint8
	Code       uint8
	Checksum   uint16
	Identifier uint16
	Seq        uint16
}

// ParseICMPEchoReply parses the ICMP echo reply messages
// This was originally based on on the the golang standard icmp
// ParseMessage, which for unknown reasons don't parse ICMP echo
// https://pkg.go.dev/golang.org/x/net/icmp#ParseMessage
// https://github.com/golang/net/blob/7fd8e65b6420/icmp/message.go#L139
func ParseICMPEchoReply(b []byte) (*ICMPEchoReply, error) {

	if len(b) < 8 {
		return nil, errMessageTooShort
	}
	er := &ICMPEchoReply{}
	br := bytes.NewReader(b)
	err := binary.Read(br, binary.BigEndian, er)
	if err != nil {
		return nil, err
	}

	return er, nil
}

// ParseICMPEchoReplyBB is the same as ParseICMPEchoReply, except
// uses bytes.Buffer, instead of []byte
// This is mostly to allow use of sync.Pool, which should be faster (maybe?)
// https://www.akshaydeo.com/blog/2017/12/23/How-did-I-improve-latency-by-700-percent-using-syncPool/
func ParseICMPEchoReplyBB(b bytes.Buffer) (*ICMPEchoReply, error) {

	if b.Len() < 8 {
		return nil, errMessageTooShort
	}
	er := &ICMPEchoReply{}
	br := bytes.NewReader(b.Bytes())
	err := binary.Read(br, binary.BigEndian, er)
	if err != nil {
		return nil, err
	}

	return er, nil
}

// IPv4
// https://tools.ietf.org/html/rfc792#page-14

// Echo or Echo Reply Message

//    0                   1                   2                   3
//    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//    |     Type      |     Code      |          Checksum             |
//    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//    |           Identifier          |        Sequence Number        |
//    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//    |     Data ...
//    +-+-+-+-+-

// IPv6
// https://tools.ietf.org/html/rfc2463#page-12

// 4.2 Echo Reply Message

//    0                   1                   2                   3
//    0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
//    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//    |     Type      |     Code      |          Checksum             |
//    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//    |           Identifier          |        Sequence Number        |
//    +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
//    |     Data ...
//    +-+-+-+-+-
