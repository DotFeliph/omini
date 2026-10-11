package dhcpcheck

import (
	"encoding/binary"
	"net"
	"slices"
	"testing"
)

// offer builds a DHCPOFFER as a server sends it.
func offer(xid uint32, options ...byte) []byte {
	b := make([]byte, 240)
	b[0], b[1], b[2] = 2, 1, 6
	binary.BigEndian.PutUint32(b[4:], xid)
	copy(b[16:], []byte{192, 168, 1, 108}) // yiaddr
	copy(b[236:], magicCookie)
	return append(append(b, options...), 255)
}

func TestDiscoverAsksForABroadcastOffer(t *testing.T) {
	mac, _ := net.ParseMAC("00:e0:4c:68:08:a3")
	b := Discover(0xcafe, mac)
	if b[0] != 1 || binary.BigEndian.Uint32(b[4:]) != 0xcafe || b[10]&0x80 == 0 {
		t.Fatalf("header: % x", b[:12])
	}
	if !slices.Equal(b[28:34], []byte(mac)) || string(b[236:240]) != string(magicCookie) {
		t.Fatal("client MAC or cookie")
	}
	if b[240] != 53 || b[242] != msgDiscover {
		t.Fatalf("message type: % x", b[240:243])
	}
}

func TestParseOffer(t *testing.T) {
	b := offer(7,
		53, 1, msgOffer,
		54, 4, 192, 168, 1, 1,
		3, 4, 192, 168, 1, 1,
		6, 8, 192, 168, 1, 1, 1, 1, 1, 1,
		15, 8, 'h', 'o', 'm', 'e', '.', 'l', 'a', 'n',
		51, 4, 0, 0, 0x0e, 0x10,
	)
	o, ok := ParseOffer(b, 7)
	if !ok || o.ServerIP != "192.168.1.1" || *o.OfferedIP != "192.168.1.108" {
		t.Fatalf("offer: %+v %v", o, ok)
	}
	if !slices.Equal(o.Dns, []string{"192.168.1.1", "1.1.1.1"}) || !slices.Equal(o.Routers, []string{"192.168.1.1"}) ||
		*o.Domain != "home.lan" || *o.LeaseS != 3600 {
		t.Fatalf("options: %+v", o)
	}
	if _, ok := ParseOffer(b, 8); ok {
		t.Fatal("another transaction")
	}
	if _, ok := ParseOffer(offer(7, 53, 1, 5), 7); ok { // an ACK
		t.Fatal("not an offer")
	}
	if _, ok := ParseOffer(b[:100], 7); ok {
		t.Fatal("truncated")
	}
}

func TestUDPPayloadOfAFrame(t *testing.T) {
	data := []byte("bootp")
	f := make([]byte, 14+20+8, 64)
	copy(f[6:12], []byte{0x58, 0x9c, 0xfc, 0x10, 0x8f, 0x2c})
	binary.BigEndian.PutUint16(f[12:], 0x0800)
	ip := f[14:]
	ip[0], ip[9] = 0x45, 17
	binary.BigEndian.PutUint16(ip[2:], uint16(20+8+len(data)))
	binary.BigEndian.PutUint16(ip[22:], 68)
	f = append(f, data...)
	f = append(f, 0, 0, 0) // Ethernet padding
	payload, src, ok := udpPayload(f)
	if !ok || string(payload) != "bootp" || src[0] != 0x58 {
		t.Fatalf("%q % x %v", payload, src, ok)
	}
	binary.BigEndian.PutUint16(f[14+22:], 67)
	if _, _, ok := udpPayload(f); ok {
		t.Fatal("port 67 is not for the client")
	}
}
