package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/riccardoalv/omini/internal/model"
)

// ErrInvalidVlan is returned for an id outside 1-4094, a name too long, a
// subnet that is not a network or a network that does not hold together.
var ErrInvalidVlan = errors.New("invalid VLAN")

// Vlan is one VLAN of the network's catalog.
type Vlan struct {
	ID     uint16 `json:"id"`
	Name   string `json:"name"`
	Color  string `json:"color,omitempty"`
	Subnet string `json:"subnet,omitempty"` // e.g. 192.168.20.0/24
	// Network is the VLAN's network on the router (gateway, DHCP, access);
	// nil when it has none.
	Network   *model.Network `json:"network,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Vlans returns the catalog, by id.
func (s *Store) Vlans(ctx context.Context) ([]Vlan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, subnet, network, created_at FROM vlans ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vlan{}
	for rows.Next() {
		var v Vlan
		var network string
		var at int64
		if err := rows.Scan(&v.ID, &v.Name, &v.Color, &v.Subnet, &network, &at); err != nil {
			return nil, err
		}
		if network != "" {
			v.Network = &model.Network{}
			if err := json.Unmarshal([]byte(network), v.Network); err != nil {
				return nil, fmt.Errorf("VLAN %d: %w", v.ID, err)
			}
		}
		v.CreatedAt = time.Unix(at, 0)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveVlan creates a VLAN of the catalog or changes it. With a network, its
// subnet is the gateway's.
func (s *Store) SaveVlan(ctx context.Context, v Vlan) (Vlan, error) {
	v.Name, v.Color, v.Subnet = strings.TrimSpace(v.Name), strings.TrimSpace(v.Color), strings.TrimSpace(v.Subnet)
	if v.ID < 1 || v.ID > 4094 {
		return v, fmt.Errorf("%w: the id is 1 to 4094", ErrInvalidVlan)
	}
	if utf8.RuneCountInString(v.Name) > 32 {
		return v, fmt.Errorf("%w: at most 32 characters in the name", ErrInvalidVlan)
	}
	network := ""
	if v.Network != nil {
		v.Network.Vlan = v.ID
		prefix, err := CheckNetwork(*v.Network)
		if err != nil {
			return v, err
		}
		v.Subnet = prefix.Masked().String()
		b, err := json.Marshal(v.Network)
		if err != nil {
			return v, err
		}
		network = string(b)
	}
	if v.Subnet != "" {
		p, err := netip.ParsePrefix(v.Subnet)
		if err != nil || p.Masked() != p {
			return v, fmt.Errorf("%w: the subnet is a network such as 192.168.20.0/24", ErrInvalidVlan)
		}
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO vlans (id, name, color, subnet, network, created_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, color = excluded.color,
			subnet = excluded.subnet, network = excluded.network`,
		v.ID, v.Name, v.Color, v.Subnet, network, v.CreatedAt.Unix())
	return v, err
}

// CheckNetwork checks that a network holds together: a private gateway
// address with its prefix (not the network's or broadcast address), a DHCP
// range inside it and without the gateway, a known access. It returns the
// gateway's prefix.
func CheckNetwork(n model.Network) (netip.Prefix, error) {
	bad := func(msg string) (netip.Prefix, error) {
		return netip.Prefix{}, fmt.Errorf("%w: %s", ErrInvalidVlan, msg)
	}
	gw, err := netip.ParsePrefix(model.Deref(n.Gateway))
	if err != nil || !gw.Addr().Is4() || gw.Bits() < 16 || gw.Bits() > 30 {
		return bad("the gateway is an IPv4 address with its prefix, /16 to /30, such as 192.168.30.1/24")
	}
	if !gw.Addr().IsPrivate() {
		return bad("the gateway is a private address (10.x, 172.16-31.x, 192.168.x)")
	}
	if gw.Addr() == gw.Masked().Addr() || gw.Addr() == LastAddr(gw) {
		return bad("the gateway cannot be the network's first or last address")
	}
	if d := n.Dhcp; d != nil && d.Enabled {
		from, err1 := netip.ParseAddr(model.Deref(d.From))
		to, err2 := netip.ParseAddr(model.Deref(d.To))
		if err1 != nil || err2 != nil || !gw.Contains(from) || !gw.Contains(to) || to.Less(from) {
			return bad("the DHCP range is two addresses of the network, the first before the last")
		}
		if from == gw.Masked().Addr() || to == LastAddr(gw) {
			return bad("the DHCP range cannot hold the network's first or last address")
		}
		if !gw.Addr().Less(from) && !to.Less(gw.Addr()) {
			return bad("the DHCP range cannot hold the gateway")
		}
	}
	switch model.Deref(n.Access) {
	case "", model.NetworkAccessInternet, model.NetworkAccessAll, model.NetworkAccessLocal, model.NetworkAccessCustom:
	default:
		return bad("the access is internet, all or local")
	}
	return gw, nil
}

// LastAddr is a prefix's broadcast address.
func LastAddr(p netip.Prefix) netip.Addr {
	a := p.Masked().Addr().As4()
	host := uint32(1)<<(32-p.Bits()) - 1
	v := (uint32(a[0])<<24 | uint32(a[1])<<16 | uint32(a[2])<<8 | uint32(a[3])) | host
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

// DeleteVlan removes a VLAN from the catalog.
func (s *Store) DeleteVlan(ctx context.Context, id uint16) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM vlans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
