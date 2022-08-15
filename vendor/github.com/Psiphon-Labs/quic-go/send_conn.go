package quic

import (
	"net"
	"time"

	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/errors"
)

const (
	UDP_PACKET_WRITE_TIMEOUT = 1 * time.Second
)

// A sendConn allows sending using a simple Write() on a non-connected packet conn.
type sendConn interface {
	Write([]byte) error
	Close() error
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

type sconn struct {
	connection

	remoteAddr net.Addr
	info       *packetInfo
	oob        []byte
}

var _ sendConn = &sconn{}

func newSendConn(c connection, remote net.Addr, info *packetInfo) sendConn {
	return &sconn{
		connection: c,
		remoteAddr: remote,
		info:       info,
		oob:        info.OOB(),
	}
}

func (c *sconn) Write(p []byte) error {
	_, err := c.WritePacket(p, c.remoteAddr, c.oob)
	return err
}

func (c *sconn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *sconn) LocalAddr() net.Addr {
	addr := c.connection.LocalAddr()
	if c.info != nil {
		if udpAddr, ok := addr.(*net.UDPAddr); ok {
			addrCopy := *udpAddr
			addrCopy.IP = c.info.addr
			addr = &addrCopy
		}
	}
	return addr
}

type spconn struct {
	net.PacketConn

	remoteAddr net.Addr
}

var _ sendConn = &spconn{}

func newSendPconn(c net.PacketConn, remote net.Addr) sendConn {
	return &spconn{PacketConn: c, remoteAddr: remote}
}

func (c *spconn) Write(p []byte) error {
	// Generally, a UDP packet write doesn't block. However, Go's
	// internal/poll.FD.WriteMsg, or internal/poll.FD.WriteTo, continue to loop
	// when syscall.SendmsgN, or syscall.Sendto, fail with EAGAIN, which
	// indicates that an OS socket buffer is currently full; in certain OS
	// states this may cause WriteTo to block indefinitely. In this scenario,
	// we want to instead behave as if the packet were dropped, so we set a
	// write deadline which will eventually interrupt any EAGAIN loop. Note
	// that quic-go manages read deadlines; we set only the write deadline here.
	err := c.SetWriteDeadline(time.Now().Add(UDP_PACKET_WRITE_TIMEOUT))
	if err != nil {
		return errors.Trace(err)
	}

	_, err = c.WriteTo(p, c.remoteAddr)
	return err
}

func (c *spconn) RemoteAddr() net.Addr {
	return c.remoteAddr
}
