// Package vlans plans the network's VLANs: what each device must change so a
// port gets its VLANs, from the device up to the router (pure functions of
// the map and the catalog).
package vlans

import (
	"fmt"
	"slices"

	"github.com/riccardoalv/omini/internal/model"
	"github.com/riccardoalv/omini/internal/topology"
)

// Entry is one VLAN of the catalog.
type Entry struct {
	ID     uint16
	Name   string
	Subnet string
	// Network is the VLAN's network on the router, when one serves it.
	Network *model.Network
}

// Import builds the catalog from the VLANs the devices have: the first name
// and subnet reported for each id, and the network a router serves on it.
// The WAN's VLANs (an operator's PPPoE VLAN) are not the network's: left out.
func Import(topo topology.Topology) []Entry {
	byID := map[uint16]*Entry{}
	var ids []uint16
	wan := WanVlans(topo)
	entry := func(id uint16) *Entry {
		e := byID[id]
		if e == nil {
			e = &Entry{ID: id}
			byID[id] = e
			ids = append(ids, id)
		}
		return e
	}
	for _, n := range topo.Nodes {
		if n.Kind != topology.KindDevice || n.Device == nil {
			continue
		}
		for _, v := range n.Device.Vlans {
			if wan[v.ID] {
				continue
			}
			e := entry(v.ID)
			if e.Name == "" {
				e.Name = model.Deref(v.Name)
			}
			if e.Subnet == "" {
				e.Subnet = model.Deref(v.Subnet)
			}
		}
		for _, nw := range n.Device.Networks {
			if wan[nw.Vlan] {
				continue
			}
			e := entry(nw.Vlan)
			if e.Network == nil {
				e.Network = &nw
			}
			if e.Name == "" && nw.Vlan != 1 {
				e.Name = model.Deref(nw.Name)
			}
		}
	}
	slices.Sort(ids)
	out := make([]Entry, 0, len(ids))
	for _, id := range ids {
		out = append(out, *byID[id])
	}
	return out
}

// WanVlans are the VLANs a WAN rides on (an operator's PPPoE or IPTV VLAN):
// a VLAN interface that is a WAN, or that a WAN interface sits on.
func WanVlans(topo topology.Topology) map[uint16]bool {
	out := map[uint16]bool{}
	for _, n := range topo.Nodes {
		if n.Device == nil {
			continue
		}
		wanOn := map[string]bool{}
		for _, i := range n.Device.Interfaces {
			if model.Deref(i.Wan) {
				wanOn[i.Name] = true
				if p := model.Deref(i.Parent); p != "" {
					wanOn[p] = true
				}
			}
		}
		for _, i := range n.Device.Interfaces {
			if v := model.Deref(i.Vlan); v != 0 && wanOn[i.Name] {
				out[v] = true
			}
		}
		for _, v := range n.Device.Vlans {
			if wanOn[model.Deref(v.Interface)] {
				out[v.ID] = true
			}
		}
	}
	return out
}

// Networks are the networks the routers serve, by VLAN (the first router's).
func Networks(topo topology.Topology) map[uint16]model.Network {
	out := map[uint16]model.Network{}
	wan := WanVlans(topo)
	for _, n := range topo.Nodes {
		if n.Kind != topology.KindDevice || n.Device == nil {
			continue
		}
		for _, nw := range n.Device.Networks {
			if _, ok := out[nw.Vlan]; !ok && !wan[nw.Vlan] {
				out[nw.Vlan] = nw
			}
		}
	}
	return out
}

// Step is one change of a plan, on one device.
type Step struct {
	NodeID string           `json:"node_id"`
	Kind   model.ChangeKind `json:"kind"`
	Port   string           `json:"port,omitempty"`
	Value  any              `json:"value"`
}

// Note is something the plan cannot do: a device on the way Omini does not
// manage (or not yet: the router), which the user configures by hand.
type Note struct {
	NodeID string   `json:"node_id"`
	Reason string   `json:"reason"` // "unmanaged", "router", "unknown_port"
	Vlans  []uint16 `json:"vlans,omitempty"`
}

// VlanSet is the value of a vlan.set step.
type VlanSet struct {
	ID   uint16 `json:"id"`
	Name string `json:"name,omitempty"`
}

// Plan is what puts port of node in want's VLANs: create the VLANs missing on
// the device, set the port, then, up the map to the router, create them on
// each switch on the way and tag them on the link between each pair.
func Plan(topo topology.Topology, catalog []Entry, nodeID, port string, want model.PortVlans) ([]Step, []Note, error) {
	byID := map[string]*topology.Node{}
	for i := range topo.Nodes {
		byID[topo.Nodes[i].ID] = &topo.Nodes[i]
	}
	names := map[uint16]string{}
	entries := map[uint16]Entry{}
	for _, e := range catalog {
		names[e.ID] = e.Name
		entries[e.ID] = e
	}
	need := slices.Clone(want.Tagged)
	if want.Untagged != nil {
		need = append(need, *want.Untagged)
	}
	slices.Sort(need)
	need = slices.Compact(need)
	for _, id := range need {
		if _, ok := names[id]; !ok {
			return nil, nil, fmt.Errorf("VLAN %d is not in the network's VLANs", id)
		}
	}
	n := byID[nodeID]
	if n == nil || n.Device == nil || !slices.Contains(n.Changes, model.ChangeKindPortVlans) {
		return nil, nil, fmt.Errorf("this device cannot change a port's VLANs")
	}
	if portOf(n, port) == nil {
		return nil, nil, fmt.Errorf("%s has no port %q", n.Label, port)
	}

	var steps []Step
	var notes []Note
	create := func(d *topology.Node) bool {
		// A device that keeps no list of VLANs (a VM's virtual NIC) takes any tag.
		if d.Device != nil && len(d.Device.Vlans) == 0 && !slices.Contains(d.Changes, model.ChangeKindVlanSet) {
			return true
		}
		var missing []uint16
		for _, id := range need {
			if !hasVlan(d, id) {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 && !slices.Contains(d.Changes, model.ChangeKindVlanSet) {
			notes = append(notes, Note{NodeID: d.ID, Reason: "unmanaged", Vlans: missing})
			return false
		}
		for _, id := range missing {
			steps = append(steps, Step{NodeID: d.ID, Kind: model.ChangeKindVlanSet, Value: VlanSet{ID: id, Name: names[id]}})
		}
		return true
	}
	// tag adds the VLANs to a port that carries them on to the next device.
	tag := func(d *topology.Node, name string) {
		p := portOf(d, name)
		if name == "" || p == nil || p.Vlans == nil || !slices.Contains(d.Changes, model.ChangeKindPortVlans) {
			notes = append(notes, Note{NodeID: d.ID, Reason: "unknown_port", Vlans: need})
			return
		}
		next := model.PortVlans{Untagged: p.Vlans.Untagged, Tagged: slices.Clone(p.Vlans.Tagged)}
		for _, id := range need {
			if model.Deref(next.Untagged) != id && !slices.Contains(next.Tagged, id) {
				next.Tagged = append(next.Tagged, id)
			}
		}
		slices.Sort(next.Tagged)
		if !slices.Equal(next.Tagged, p.Vlans.Tagged) {
			steps = append(steps, Step{NodeID: d.ID, Kind: model.ChangeKindPortVlans, Port: name, Value: next})
		}
	}

	if !create(n) {
		return steps, notes, nil
	}
	steps = append(steps, Step{NodeID: n.ID, Kind: model.ChangeKindPortVlans, Port: port, Value: want})
	// tagUp tags a device's port toward its parent: never the port just set (a
	// VM's only NIC), nor on a device Omini cannot change (passed through).
	tagUp := func(cur *topology.Node, up string) {
		if cur.ID == n.ID && (up == port || up == "") && len(cur.Device.Vlans) == 0 {
			return
		}
		if cur.Device == nil || !slices.Contains(cur.Changes, model.ChangeKindPortVlans) {
			return
		}
		tag(cur, up)
	}
	for cur, seen := n, map[string]bool{n.ID: true}; ; {
		edge := uplink(topo, cur.ID)
		if edge == nil || seen[edge.Source] {
			break
		}
		parent := byID[edge.Source]
		if parent == nil {
			break
		}
		seen[parent.ID] = true
		up := uplinkPort(cur, parent, edge.TargetPort)
		if isRouter(parent) {
			// The router serves the VLAN's network: Omini creates it there
			// when the catalog has one and the router can, else it is a note.
			var missing []uint16
			for _, id := range need {
				if id == 1 || hasVlan(parent, id) || hasNetwork(parent, id) {
					continue
				}
				e := entries[id]
				if e.Network == nil || !slices.Contains(parent.Changes, model.ChangeKindNetworkSet) {
					missing = append(missing, id)
					continue
				}
				steps = append(steps, Step{NodeID: parent.ID, Kind: model.ChangeKindNetworkSet, Value: NetworkFor(e, edge.SourcePort)})
			}
			if len(missing) > 0 {
				notes = append(notes, Note{NodeID: parent.ID, Reason: "router", Vlans: missing})
			}
			tagUp(cur, up)
			break
		}
		if parent.Kind != topology.KindDevice || parent.Device == nil || !slices.Contains(parent.Changes, model.ChangeKindPortVlans) {
			// Omini cannot change it (a hypervisor's bridge, a dumb switch): a
			// note, and the way goes on above it.
			notes = append(notes, Note{NodeID: parent.ID, Reason: "unmanaged", Vlans: need})
			tagUp(cur, up)
			cur = parent
			continue
		}
		tagUp(cur, up)
		if !create(parent) {
			break
		}
		tag(parent, edge.SourcePort)
		cur = parent
	}
	return steps, notes, nil
}

func uplink(topo topology.Topology, id string) *topology.Edge {
	for i, e := range topo.Edges {
		if e.Target == id && e.Kind != topology.EdgeVPN {
			return &topo.Edges[i]
		}
	}
	return nil
}

func portOf(n *topology.Node, name string) *model.Interface {
	if n.Device == nil {
		return nil
	}
	for i := range n.Device.Interfaces {
		if n.Device.Interfaces[i].Name == name {
			return &n.Device.Interfaces[i]
		}
	}
	return nil
}

func hasVlan(n *topology.Node, id uint16) bool {
	return n.Device != nil && slices.ContainsFunc(n.Device.Vlans, func(v model.Vlan) bool { return v.ID == id })
}

func hasNetwork(n *topology.Node, id uint16) bool {
	return n.Device != nil && slices.ContainsFunc(n.Device.Networks, func(x model.Network) bool { return x.Vlan == id })
}

// NetworkFor is the network.set value of a catalog VLAN: its network, named
// after the VLAN, on the router's port toward the switches (parent) unless
// it names its own.
func NetworkFor(e Entry, parent string) model.Network {
	n := model.Network{Vlan: e.ID}
	if e.Network != nil {
		n = *e.Network
		n.Vlan = e.ID
	}
	if model.Deref(n.Name) == "" && e.Name != "" {
		n.Name = &e.Name
	}
	if model.Deref(n.Parent) == "" && parent != "" {
		n.Parent = &parent
	}
	return n
}

func isRouter(n *topology.Node) bool {
	return n.Role == string(model.DeviceRoleRouter) || n.Role == string(model.DeviceRoleFirewall)
}

// uplinkPort is a device's port towards its parent: the link's, else the
// port where the device learned one of the parent's MACs (an inferred link
// names no port on this end).
func uplinkPort(cur, parent *topology.Node, port string) string {
	if port != "" || cur.Device == nil {
		return port
	}
	macs := []model.MACAddress{model.MACAddress(parent.MAC)}
	if parent.Device != nil {
		macs = append(macs, parent.Device.MACs...)
	}
	for _, f := range cur.Device.Fdb {
		if slices.Contains(macs, f.MAC) {
			return f.Port
		}
	}
	return ""
}
