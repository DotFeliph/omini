package dnscheck

import (
	"context"
	"encoding/binary"
	"net"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// serve answers A queries for one name from a UDP socket on 127.0.0.1, and
// NXDOMAIN for the rest; Port points queries at it.
func serve(t *testing.T, name string, ip [4]byte) {
	t.Helper()
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	old := Port
	Port = strconv.Itoa(pc.LocalAddr().(*net.UDPAddr).Port)
	t.Cleanup(func() { Port = old })
	go func() {
		buf := make([]byte, 512)
		for {
			n, addr, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			q := buf[:n]
			// The question: labels from byte 12, then its type and class.
			end := 12
			var labels []string
			for end < len(q) && q[end] != 0 {
				l := int(q[end])
				labels = append(labels, string(q[end+1:end+1+l]))
				end += 1 + l
			}
			end += 5
			resp := append([]byte(nil), q[:end]...)
			resp[2] |= 0x80                          // a response
			binary.BigEndian.PutUint16(resp[10:], 0) // no additional records
			qtype := binary.BigEndian.Uint16(q[end-4:])
			switch {
			case strings.Join(labels, ".") != name:
				resp[3] |= 3 // NXDOMAIN
			case qtype == 1:
				binary.BigEndian.PutUint16(resp[6:], 1) // one answer
				resp = append(resp, 0xc0, 12, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4)
				resp = append(resp, ip[:]...)
			}
			_, _ = pc.WriteTo(resp, addr)
		}
	}()
}

func TestQuery(t *testing.T) {
	serve(t, "nas.home.lan", [4]byte{192, 168, 1, 10})
	q := Query(context.Background(), "127.0.0.1", "nas.home.lan")
	if !q.Ok || !slices.Equal(q.Answers, []string{"192.168.1.10"}) || q.RttMs == nil || q.Error != nil {
		t.Fatalf("found: %+v", q)
	}
	q = Query(context.Background(), "127.0.0.1", "missing.home.lan")
	if q.Ok || q.Error == nil || *q.Error != "not found" {
		t.Fatalf("missing: %+v", q)
	}
}

func TestQueryWithoutAServer(t *testing.T) {
	pc, _ := net.ListenPacket("udp4", "127.0.0.1:0")
	old := Port
	Port = strconv.Itoa(pc.LocalAddr().(*net.UDPAddr).Port)
	pc.Close() // nothing listens there now
	defer func() { Port = old }()
	q := Query(context.Background(), "127.0.0.1", "example.com")
	if q.Ok || q.Error == nil || (*q.Error != "refused" && *q.Error != "timeout") {
		t.Fatalf("no server: %+v", q)
	}
}
