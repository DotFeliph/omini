package netscan

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"github.com/riccardoalv/omini/internal/integration"
	"github.com/riccardoalv/omini/internal/model"
	"github.com/riccardoalv/omini/internal/snmp"
)

// agent is where an SNMP device was read: its address and the credential it
// answered to (a community, or snmp.V3Credential).
type agent struct {
	ip         netip.Addr
	credential string
}

// Apply enables or disables a port of an SNMP device (IF-MIB ifAdminStatus),
// with the write community, or with the v3 user when the device answers v3.
// It returns the device read again after the change.
func (s *Integration) Apply(ctx context.Context, cfg integration.Config, change model.Change) ([]model.Device, error) {
	enabled, ok := change.Value.(bool)
	if change.Kind != model.ChangeKindPortEnabled || !ok || change.Port == nil {
		return nil, fmt.Errorf("the network scan cannot apply %s", change.Kind)
	}
	opts, err := s.options(cfg)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	a, known := s.agents[change.Device]
	s.mu.Unlock()
	if !known {
		return nil, errors.New("this device has not been read by SNMP since Omini started: try again after the next round")
	}
	write := s.snmpTarget(a.ip, opts.writeCommunity, nil)
	switch {
	case a.credential == snmp.V3Credential && opts.v3 != nil:
		write = s.snmpTarget(a.ip, snmp.V3Credential, opts.v3)
	case opts.writeCommunity == "":
		return nil, errors.New("set the SNMP write community in the network scan's settings (or use an SNMP v3 user allowed to write)")
	}
	if err := snmp.SetPortEnabled(ctx, write, *change.Port, enabled); err != nil {
		if errors.Is(err, snmp.ErrNoWrite) {
			return nil, fmt.Errorf("%w: check the write community in the network scan's settings, or the v3 user's write view", err)
		}
		return nil, err
	}
	d, err := snmp.Collect(ctx, s.snmpTarget(a.ip, a.credential, opts.v3))
	if err != nil {
		return nil, fmt.Errorf("the port was changed, but reading the device again failed: %w", err)
	}
	return []model.Device{d}, nil
}
