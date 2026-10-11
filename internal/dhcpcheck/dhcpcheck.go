// Package dhcpcheck checks the DHCP servers of a network: it broadcasts a
// DHCPDISCOVER and collects the offers. It never sends a REQUEST, so no
// address is taken; the client MAC is this machine's own, so a server offers
// the address it already has instead of one of its pool.
package dhcpcheck

import (
	"encoding/binary"
	"net"
	"net/netip"
	"time"

	"github.com/riccardoalv/omini/internal/model"
)

const (
	msgDiscover = 1
	msgOffer    = 2
	// Wait for offers this long after the DISCOVER (servers answer in ms; some
	// wait to check the address first).
	Wait = 3 * time.Second
)

var magicCookie = []byte{99, 130, 83, 99}

// Discover is a DHCPDISCOVER from mac with transaction id xid, asking for a
// broadcast answer (so it reaches a client without an address too).
func Discover(xid uint32, mac net.HardwareAddr) []byte {
	b := make([]byte, 240, 300)
	b[0], b[1], b[2] = 1, 1, 6 // BOOTREQUEST, Ethernet, 6-byte MAC
	binary.BigEndian.PutUint32(b[4:], xid)
	binary.BigEndian.PutUint16(b[10:], 0x8000) // broadcast flag
	copy(b[28:], mac)
	copy(b[236:], magicCookie)
	b = append(b,
		53, 1, msgDiscover,
		55, 6, 1, 3, 6, 15, 51, 54, // ask: mask, router, DNS, domain, lease, server id
		255)
	return b
}

// ParseOffer reads a DHCPOFFER for xid from a BOOTP payload (the UDP data).
func ParseOffer(b []byte, xid uint32) (model.DhcpOffer, bool) {
	var o model.DhcpOffer
	if len(b) < 240 || b[0] != 2 || binary.BigEndian.Uint32(b[4:]) != xid || string(b[236:240]) != string(magicCookie) {
		return o, false
	}
	if yi, ok := netip.AddrFromSlice(b[16:20]); ok && !yi.IsUnspecified() {
		o.OfferedIP = model.Ptr(yi.String())
	}
	if si, ok := netip.AddrFromSlice(b[20:24]); ok && !si.IsUnspecified() {
		o.ServerIP = si.String()
	}
	var msgType byte
	for i := 240; i < len(b); {
		code := b[i]
		if code == 255 {
			break
		}
		if code == 0 {
			i++
			continue
		}
		if i+1 >= len(b) || i+2+int(b[i+1]) > len(b) {
			break
		}
		val := b[i+2 : i+2+int(b[i+1])]
		switch code {
		case 53:
			if len(val) == 1 {
				msgType = val[0]
			}
		case 54:
			if a, ok := netip.AddrFromSlice(val); ok && len(val) == 4 {
				o.ServerIP = a.String()
			}
		case 3:
			o.Routers = addrs(val)
		case 6:
			o.Dns = addrs(val)
		case 15:
			if d := string(val); d != "" {
				o.Domain = model.Ptr(d)
			}
		case 51:
			if len(val) == 4 {
				o.LeaseS = model.Ptr(uint64(binary.BigEndian.Uint32(val)))
			}
		}
		i += 2 + len(val)
	}
	return o, msgType == msgOffer && o.ServerIP != ""
}

func addrs(b []byte) []string {
	var out []string
	for i := 0; i+4 <= len(b); i += 4 {
		a, _ := netip.AddrFromSlice(b[i : i+4])
		out = append(out, a.String())
	}
	return out
}
