//go:build !linux

package dhcpcheck

import (
	"context"
	"errors"
	"net"

	"github.com/riccardoalv/omini/internal/model"
)

// Probe needs Linux packet sockets.
func Probe(context.Context, string, net.HardwareAddr) ([]model.DhcpOffer, error) {
	return nil, errors.New("the DHCP check needs Linux")
}
