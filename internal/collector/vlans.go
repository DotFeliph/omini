package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/riccardoalv/omini/internal/model"
	"github.com/riccardoalv/omini/internal/store"
	"github.com/riccardoalv/omini/internal/topology"
	"github.com/riccardoalv/omini/internal/vlans"
)

// VlansImportedSetting records that the catalog was imported from the devices
// once: emptied by hand, it is not imported again.
const VlansImportedSetting = "vlans_imported"

// Vlans is the network's VLAN catalog. The first time, it is imported from
// the VLANs the devices have.
func (c *Collector) Vlans(ctx context.Context) ([]store.Vlan, error) {
	list, err := c.store.Vlans(ctx)
	if err != nil || len(list) > 0 {
		return list, err
	}
	if _, done, err := c.store.GetSetting(ctx, VlansImportedSetting); err != nil || done {
		return list, err
	}
	found := vlans.Import(c.State().Topology)
	if len(found) == 0 {
		return list, nil // nothing to import yet: try again once devices report VLANs
	}
	for _, e := range found {
		if _, err := c.store.SaveVlan(ctx, store.Vlan{ID: e.ID, Name: e.Name, Subnet: e.Subnet}); err != nil {
			return nil, err
		}
	}
	if err := c.store.SetSetting(ctx, VlansImportedSetting, "1"); err != nil {
		return nil, err
	}
	return c.store.Vlans(ctx)
}

// PlanStep is a step of a VLAN plan, with its protection when destructive.
type PlanStep struct {
	vlans.Step
	Protected *topology.Protection `json:"protected,omitempty"`
}

// VlanPlan is what a port's VLANs change on every device.
type VlanPlan struct {
	Steps []PlanStep   `json:"steps"`
	Notes []vlans.Note `json:"notes"`
}

// PlanVlans is what setting a port's VLANs changes on the way to the router.
func (c *Collector) PlanVlans(ctx context.Context, nodeID, port string, want model.PortVlans) (VlanPlan, error) {
	catalog, err := c.Vlans(ctx)
	if err != nil {
		return VlanPlan{}, err
	}
	entries := make([]vlans.Entry, len(catalog))
	for i, v := range catalog {
		entries[i] = vlans.Entry{ID: v.ID, Name: v.Name, Subnet: v.Subnet}
	}
	topo := c.State().Topology
	steps, notes, err := vlans.Plan(topo, entries, nodeID, port, want)
	if err != nil {
		return VlanPlan{}, fmt.Errorf("%w: %w", ErrInvalidChange, err)
	}
	plan := VlanPlan{Steps: make([]PlanStep, len(steps)), Notes: notes}
	if plan.Notes == nil {
		plan.Notes = []vlans.Note{}
	}
	for i, s := range steps {
		plan.Steps[i] = PlanStep{Step: s, Protected: protectedStep(topo, s)}
	}
	return plan, nil
}

// protectedStep is the protection a step would cross, when it is destructive.
func protectedStep(topo topology.Topology, s vlans.Step) *topology.Protection {
	i := slices.IndexFunc(topo.Nodes, func(n topology.Node) bool { return n.ID == s.NodeID })
	if i < 0 || s.Port == "" {
		return nil
	}
	n := topo.Nodes[i]
	p, guarded := n.Protected[s.Port]
	if !guarded || n.Device == nil {
		return nil
	}
	var before json.RawMessage
	for _, iface := range n.Device.Interfaces {
		if iface.Name == s.Port && iface.Vlans != nil {
			before, _ = json.Marshal(iface.Vlans)
		}
	}
	if destructive(ChangeRequest{Kind: s.Kind, Port: s.Port, Value: s.Value}, before) {
		return &p
	}
	return nil
}

// ApplyVlans sets a port's VLANs and the way to the router, step by step,
// stopping at the first that fails. A destructive step is blocked unless
// destructive changes are allowed and the plan is confirmed.
func (c *Collector) ApplyVlans(ctx context.Context, nodeID, port string, want model.PortVlans, confirmed bool) ([]store.ConfigChange, error) {
	plan, err := c.PlanVlans(ctx, nodeID, port, want)
	if err != nil {
		return nil, err
	}
	for _, s := range plan.Steps {
		if s.Protected != nil {
			if allowed := c.AllowDestructive(ctx); !allowed || !confirmed {
				return nil, &DestructiveError{Protection: *s.Protected, Allowed: allowed}
			}
		}
	}
	done := []store.ConfigChange{}
	for _, s := range plan.Steps {
		rec, err := c.Apply(ctx, ChangeRequest{NodeID: s.NodeID, Kind: s.Kind, Port: s.Port, Value: s.Value, Confirmed: true})
		if rec.ID != 0 {
			done = append(done, rec)
		}
		if err != nil {
			return done, err
		}
	}
	return done, nil
}

// SaveVlan creates or changes a VLAN of the catalog; a new name is applied to
// every device that has the VLAN and can rename it.
func (c *Collector) SaveVlan(ctx context.Context, v store.Vlan) (store.Vlan, []store.ConfigChange, error) {
	saved, err := c.store.SaveVlan(ctx, v)
	if err != nil {
		return saved, nil, err
	}
	var done []store.ConfigChange
	for _, n := range c.State().Topology.Nodes {
		i := vlanIndex(n, v.ID)
		if i < 0 || model.Deref(n.Device.Vlans[i].Name) == saved.Name || !slices.Contains(n.Changes, model.ChangeKindVlanSet) {
			continue
		}
		rec, err := c.Apply(ctx, ChangeRequest{NodeID: n.ID, Kind: model.ChangeKindVlanSet, Value: VlanValue{ID: v.ID, Name: saved.Name}})
		if rec.ID != 0 {
			done = append(done, rec)
		}
		if err != nil {
			return saved, done, err
		}
	}
	return saved, done, nil
}

// ErrVlanOnDevices: a VLAN cannot leave the catalog while a device keeps it.
var ErrVlanOnDevices = errors.New("the VLAN is still on devices")

// DeleteVlan deletes a VLAN from every device that has it, then from the
// catalog. A device that cannot delete it, or a port that still uses it,
// stops it.
func (c *Collector) DeleteVlan(ctx context.Context, id uint16) ([]store.ConfigChange, error) {
	var done []store.ConfigChange
	for _, n := range c.State().Topology.Nodes {
		if vlanIndex(n, id) < 0 {
			continue
		}
		if !slices.Contains(n.Changes, model.ChangeKindVlanDelete) {
			return done, fmt.Errorf("%w: %s cannot delete it from Omini: delete it there", ErrVlanOnDevices, n.Label)
		}
		rec, err := c.Apply(ctx, ChangeRequest{NodeID: n.ID, Kind: model.ChangeKindVlanDelete, Value: VlanValue{ID: id}})
		if rec.ID != 0 {
			done = append(done, rec)
		}
		if err != nil {
			return done, err
		}
	}
	return done, c.store.DeleteVlan(ctx, id)
}

func vlanIndex(n topology.Node, id uint16) int {
	if n.Kind != topology.KindDevice || n.Device == nil {
		return -1
	}
	return slices.IndexFunc(n.Device.Vlans, func(v model.Vlan) bool { return v.ID == id })
}
