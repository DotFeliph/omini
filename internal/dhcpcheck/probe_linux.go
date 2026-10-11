//go:build linux

package dhcpcheck

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"

	"github.com/riccardoalv/omini/internal/model"
)

// Probe broadcasts a DHCPDISCOVER on iface (from mac, the interface's own) and
// returns the offers that arrive within Wait, one per server. The answers are
// read with a packet socket (raw sockets: root or CAP_NET_RAW), which also
// gives the server's MAC and works while this machine's own DHCP client holds
// port 68.
func Probe(ctx context.Context, iface string, mac net.HardwareAddr) ([]model.DhcpOffer, error) {
	ifc, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, err
	}
	// ETH_P_ALL: also the frames this machine sends, so an offer from a DHCP
	// server running here (Omini on the router) is seen too; the filter keeps
	// IPv4 alone.
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW|unix.SOCK_CLOEXEC, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		return nil, fmt.Errorf("packet socket: %w", err)
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrLinklayer{Protocol: htons(unix.ETH_P_ALL), Ifindex: ifc.Index}); err != nil {
		return nil, fmt.Errorf("bind %s: %w", iface, err)
	}
	if err := attachFilter(fd); err != nil {
		return nil, err
	}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Usec: 200_000}); err != nil {
		return nil, err
	}

	var x [4]byte
	_, _ = rand.Read(x[:])
	xid := binary.BigEndian.Uint32(x[:])
	if err := send(ctx, iface, Discover(xid, mac)); err != nil {
		return nil, err
	}
	sent := time.Now()

	var out []model.DhcpOffer
	seen := map[string]bool{}
	buf := make([]byte, 1600)
	for time.Since(sent) < Wait && ctx.Err() == nil {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		switch {
		case errors.Is(err, unix.EAGAIN), errors.Is(err, unix.EINTR):
			continue
		case err != nil:
			return out, err
		}
		payload, src, ok := udpPayload(buf[:n])
		if !ok {
			continue
		}
		o, ok := ParseOffer(payload, xid)
		if !ok || seen[o.ServerIP] {
			continue
		}
		seen[o.ServerIP] = true
		if m := model.MACFromBytes(src); m != "" {
			o.ServerMAC = &m
		}
		o.RttMs = model.Ptr(float64(time.Since(sent).Microseconds()) / 1000)
		out = append(out, o)
	}
	return out, ctx.Err()
}

// send broadcasts the DISCOVER from an ephemeral port out of iface (servers
// answer to port 68 whatever the source port).
func send(ctx context.Context, iface string, msg []byte) error {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			serr = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1)
			if serr == nil {
				serr = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, iface)
			}
		})
		if err != nil {
			return err
		}
		return serr
	}}
	pc, err := lc.ListenPacket(ctx, "udp4", ":0")
	if err != nil {
		return fmt.Errorf("udp socket on %s: %w", iface, err)
	}
	defer pc.Close()
	_, err = pc.WriteTo(msg, &net.UDPAddr{IP: net.IPv4bcast, Port: 67})
	return err
}

// attachFilter passes only UDP datagrams to port 68 (DHCP client).
func attachFilter(fd int) error {
	// With SOCK_RAW the frame starts at the Ethernet header (14 bytes).
	prog, err := bpf.Assemble([]bpf.Instruction{
		bpf.LoadAbsolute{Off: 12, Size: 2},                           // EtherType
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 0x0800, SkipTrue: 8}, // IPv4
		bpf.LoadAbsolute{Off: 23, Size: 1},                           // protocol
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 17, SkipTrue: 6},     // UDP
		bpf.LoadAbsolute{Off: 20, Size: 2},                           // flags + fragment offset
		bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x1fff, SkipTrue: 4},  // not a later fragment
		bpf.LoadMemShift{Off: 14},                                    // X = IP header length
		bpf.LoadIndirect{Off: 16, Size: 2},                           // UDP destination port
		bpf.JumpIf{Cond: bpf.JumpNotEqual, Val: 68, SkipTrue: 1},
		bpf.RetConstant{Val: 1600},
		bpf.RetConstant{Val: 0},
	})
	if err != nil {
		return err
	}
	filter := make([]unix.SockFilter, len(prog))
	for i, ins := range prog {
		filter[i] = unix.SockFilter{Code: ins.Op, Jt: ins.Jt, Jf: ins.Jf, K: ins.K}
	}
	if err := unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER,
		&unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}); err != nil {
		return fmt.Errorf("filter: %w", err)
	}
	return nil
}

func htons(v uint16) uint16 { return v<<8 | v>>8 }
