package store

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrInvalidVlan is returned for an id outside 1-4094, a name too long or a
// subnet that is not a network.
var ErrInvalidVlan = errors.New("invalid VLAN")

// Vlan is one VLAN of the network's catalog.
type Vlan struct {
	ID        uint16    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	Subnet    string    `json:"subnet,omitempty"` // e.g. 192.168.20.0/24
	CreatedAt time.Time `json:"created_at"`
}

// Vlans returns the catalog, by id.
func (s *Store) Vlans(ctx context.Context) ([]Vlan, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, subnet, created_at FROM vlans ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vlan{}
	for rows.Next() {
		var v Vlan
		var at int64
		if err := rows.Scan(&v.ID, &v.Name, &v.Color, &v.Subnet, &at); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(at, 0)
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveVlan creates a VLAN of the catalog or changes its name, color and subnet.
func (s *Store) SaveVlan(ctx context.Context, v Vlan) (Vlan, error) {
	v.Name, v.Color, v.Subnet = strings.TrimSpace(v.Name), strings.TrimSpace(v.Color), strings.TrimSpace(v.Subnet)
	if v.ID < 1 || v.ID > 4094 {
		return v, fmt.Errorf("%w: the id is 1 to 4094", ErrInvalidVlan)
	}
	if utf8.RuneCountInString(v.Name) > 32 {
		return v, fmt.Errorf("%w: at most 32 characters in the name", ErrInvalidVlan)
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
		INSERT INTO vlans (id, name, color, subnet, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, color = excluded.color, subnet = excluded.subnet`,
		v.ID, v.Name, v.Color, v.Subnet, v.CreatedAt.Unix())
	return v, err
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
