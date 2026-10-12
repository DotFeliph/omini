package store

import (
	"context"
	"errors"
	"testing"

	"github.com/riccardoalv/omini/internal/model"
)

func TestVlanCatalog(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	for _, v := range []Vlan{{ID: 20, Name: "Guests", Subnet: "192.168.20.0/24"}, {ID: 10, Name: " Office ", Color: "blue"}} {
		if _, err := s.SaveVlan(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.SaveVlan(ctx, Vlan{ID: 20, Name: "Visitors", Subnet: "192.168.20.0/24"}); err != nil {
		t.Fatal(err)
	}
	list, err := s.Vlans(ctx)
	if err != nil || len(list) != 2 || list[0].ID != 10 || list[0].Name != "Office" || list[1].Name != "Visitors" {
		t.Fatalf("catalog: %+v %v", list, err)
	}
	for _, bad := range []Vlan{{ID: 0}, {ID: 4095}, {ID: 30, Subnet: "192.168.30.1/24"}, {ID: 30, Subnet: "lan"}} {
		if _, err := s.SaveVlan(ctx, bad); !errors.Is(err, ErrInvalidVlan) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if err := s.DeleteVlan(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVlan(ctx, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("again: %v", err)
	}
}

func TestVlanNetwork(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	net := func(gw, from, to string, access model.NetworkAccess) *model.Network {
		return &model.Network{
			Gateway: &gw,
			Dhcp:    &model.NetworkDhcp{Enabled: from != "", From: &from, To: &to},
			Access:  &access,
		}
	}
	v, err := s.SaveVlan(ctx, Vlan{ID: 30, Name: "IOT", Network: net("192.168.30.1/24", "192.168.30.100", "192.168.30.250", "internet")})
	if err != nil || v.Subnet != "192.168.30.0/24" || v.Network.Vlan != 30 {
		t.Fatalf("saved %+v %v", v, err)
	}
	list, err := s.Vlans(ctx)
	if err != nil || list[0].Network == nil || model.Deref(list[0].Network.Gateway) != "192.168.30.1/24" ||
		model.Deref(list[0].Network.Access) != model.NetworkAccessInternet || !list[0].Network.Dhcp.Enabled {
		t.Fatalf("read back %+v %v", list, err)
	}
	for name, bad := range map[string]*model.Network{
		"no prefix":        net("192.168.30.1", "", "", ""),
		"public":           net("8.8.8.1/24", "", "", ""),
		"network address":  net("192.168.30.0/24", "", "", ""),
		"broadcast":        net("192.168.30.255/24", "", "", ""),
		"range outside":    net("192.168.30.1/24", "192.168.31.10", "192.168.31.20", ""),
		"range backwards":  net("192.168.30.1/24", "192.168.30.200", "192.168.30.100", ""),
		"range on gateway": net("192.168.30.1/24", "192.168.30.1", "192.168.30.50", ""),
		"range broadcast":  net("192.168.30.1/24", "192.168.30.100", "192.168.30.255", ""),
		"access":           net("192.168.30.1/24", "", "", "everything"),
	} {
		if _, err := s.SaveVlan(ctx, Vlan{ID: 31, Network: bad}); !errors.Is(err, ErrInvalidVlan) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A DHCP that is off needs no range.
	if _, err := s.SaveVlan(ctx, Vlan{ID: 32, Network: net("10.0.32.1/24", "", "", "local")}); err != nil {
		t.Fatal(err)
	}
}
