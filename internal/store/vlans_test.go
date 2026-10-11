package store

import (
	"context"
	"errors"
	"testing"
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
