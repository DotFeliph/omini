// Package dnscheck checks a DNS server: it resolves a name with that server
// alone and times the answer.
package dnscheck

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/riccardoalv/omini/internal/model"
)

// Timeout bounds one query; Slow is when an answer counts as slow.
const (
	Timeout = 3 * time.Second
	Slow    = 500 * time.Millisecond
)

// Port is the DNS port (tests use another).
var Port = "53"

// Query resolves name's IPv4 addresses with server only.
func Query(ctx context.Context, server, name string) model.DnsQuery {
	q := model.DnsQuery{Name: name}
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort(server, Port))
	}}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	start := time.Now()
	// Absolute: never completed with this machine's search domains.
	addrs, err := r.LookupNetIP(ctx, "ip4", strings.TrimSuffix(name, ".")+".")
	q.RttMs = model.Ptr(float64(time.Since(start).Microseconds()) / 1000)
	if err != nil {
		q.Error = model.Ptr(reason(err))
		return q
	}
	q.Ok = true
	for _, a := range addrs {
		q.Answers = append(q.Answers, a.String())
	}
	return q
}

// reason says why a query failed in a word the UI translates.
func reason(err error) string {
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return "not found"
	case errors.As(err, &dnsErr) && dnsErr.IsTimeout, errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, syscall.ECONNREFUSED), strings.Contains(err.Error(), "connection refused"):
		return "refused"
	}
	return err.Error()
}
