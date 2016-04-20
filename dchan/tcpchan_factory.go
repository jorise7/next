package dchan

import (
	"net"

	"github.com/chzyer/flow"
	"github.com/chzyer/next/packet"
	"gopkg.in/logex.v1"
)

var (
	ErrInvalidUserId        = logex.Define("invalid user id")
	ErrUnexpectedPacketType = logex.Define("unexpected packet type")
)

var _ ChannelFactory = TcpChanFactory{}

type TcpChanFactory struct{}

func (TcpChanFactory) NewClient(f *flow.Flow, session *packet.Session, conn net.Conn, out chan<- *packet.Packet) Channel {
	return NewTcpChanClient(f, session, conn, out)
}
func (TcpChanFactory) NewServer(f *flow.Flow, session *packet.Session, conn net.Conn, delegate SvrInitDelegate) Channel {
	return NewTcpChanServer(f, session, conn, delegate)
}