package netscan

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/riccardoalv/omini/internal/integration"
	"github.com/riccardoalv/omini/internal/model"
	"github.com/riccardoalv/omini/internal/snmp/snmptest"
)

// TestDisablesAPortOfAnSNMPDevice: a switch found by the scan has a port
// disabled with the write community, and comes back read again.
func TestDisablesAPortOfAnSNMPDevice(t *testing.T) {
	sw := model.Device{Key: "x", Name: "sw-core", Interfaces: []model.Interface{{Name: "ge-0/0/1", Up: model.Ptr(true)}}}
	agent := snmptest.Start(t, "homelab", snmptest.FromDevice(sw, snmptest.Options{}))
	agent.AllowWrite("homelab-rw")
	s := &Integration{
		ARPPath: "testdata/arp", RoutePath: "testdata/route", Ports: []int{},
		SettleTime: 10 * time.Millisecond, NoMulticast: true, NoPing: true, NoWebTitles: true,
		SNMPPort: agent.Port, SNMPTimeout: 300 * time.Millisecond,
		Locals: func() []localNet { return nil },
	}
	cfg := integration.Config{"subnets": "127.0.0.0/30", "snmp_communities": "homelab"}
	devices, err := s.Collect(context.Background(), cfg)
	if err != nil || len(devices) != 2 {
		t.Fatalf("scan: %v %v", devices, err)
	}
	change := model.Change{Device: devices[1].Key, Kind: model.ChangeKindPortEnabled, Port: model.Ptr("ge-0/0/1"), Value: false}

	if _, err := s.Apply(context.Background(), cfg, change); err == nil || !strings.Contains(err.Error(), "write community") {
		t.Fatalf("without a write community: %v", err)
	}
	cfg["snmp_write_community"] = "homelab"
	if _, err := s.Apply(context.Background(), cfg, change); err == nil || !strings.Contains(err.Error(), "refused to write") {
		t.Fatalf("with a read-only community: %v", err)
	}
	cfg["snmp_write_community"] = "homelab-rw"
	got, err := s.Apply(context.Background(), cfg, change)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Key != devices[1].Key || *got[0].Interfaces[0].Enabled {
		t.Fatalf("read again after the change: %+v", got)
	}
	unknown := change
	unknown.Device = "not-read"
	if _, err := s.Apply(context.Background(), cfg, unknown); err == nil {
		t.Fatal("a device the scan did not read must fail")
	}
}
