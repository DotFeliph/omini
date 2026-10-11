package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"

	"github.com/riccardoalv/omini/internal/collector"
	"github.com/riccardoalv/omini/internal/model"
	"github.com/riccardoalv/omini/internal/store"
	"github.com/riccardoalv/omini/internal/topology"
)

// The network's VLANs: the catalog, where each one is, and the plans that put
// a port in VLANs (with the way to the router).

type vlanView struct {
	store.Vlan
	Devices []vlanOnDevice `json:"devices"`
}

type vlanOnDevice struct {
	NodeID   string   `json:"node_id"`
	Name     string   `json:"name,omitempty"` // as the device names it
	Untagged []string `json:"untagged"`       // its ports in the VLAN untagged (access, native)
	Tagged   []string `json:"tagged"`
}

func (s *Server) listVlans(w http.ResponseWriter, r *http.Request) {
	catalog, err := s.Collector.Vlans(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	topo := s.Collector.State().Topology
	out := make([]vlanView, len(catalog))
	for i, v := range catalog {
		out[i] = vlanView{Vlan: v, Devices: vlanDevices(topo, v.ID)}
	}
	writeJSON(w, http.StatusOK, out)
}

func vlanDevices(topo topology.Topology, id uint16) []vlanOnDevice {
	out := []vlanOnDevice{}
	for _, n := range topo.Nodes {
		if n.Kind != topology.KindDevice || n.Device == nil {
			continue
		}
		i := slices.IndexFunc(n.Device.Vlans, func(v model.Vlan) bool { return v.ID == id })
		if i < 0 {
			continue
		}
		d := vlanOnDevice{NodeID: n.ID, Name: model.Deref(n.Device.Vlans[i].Name), Untagged: []string{}, Tagged: []string{}}
		for _, p := range n.Device.Interfaces {
			switch {
			case p.Vlans == nil:
			case model.Deref(p.Vlans.Untagged) == id:
				d.Untagged = append(d.Untagged, p.Name)
			case slices.Contains(p.Vlans.Tagged, id):
				d.Tagged = append(d.Tagged, p.Name)
			}
		}
		out = append(out, d)
	}
	return out
}

func vlanID(w http.ResponseWriter, r *http.Request) (uint16, bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 16)
	if err != nil || id < 1 || id > 4094 {
		writeError(w, http.StatusBadRequest, "a VLAN id is 1 to 4094")
		return 0, false
	}
	return uint16(id), true
}

func (s *Server) createVlan(w http.ResponseWriter, r *http.Request) {
	var in store.Vlan
	if !readJSON(w, r, &in) {
		return
	}
	catalog, err := s.Collector.Vlans(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	if slices.ContainsFunc(catalog, func(v store.Vlan) bool { return v.ID == in.ID }) {
		writeError(w, http.StatusConflict, "the network already has this VLAN")
		return
	}
	s.saveVlan(w, r, in, http.StatusCreated)
}

func (s *Server) updateVlan(w http.ResponseWriter, r *http.Request) {
	id, ok := vlanID(w, r)
	if !ok {
		return
	}
	catalog, err := s.Collector.Vlans(r.Context())
	if err != nil {
		internalError(w, err)
		return
	}
	i := slices.IndexFunc(catalog, func(v store.Vlan) bool { return v.ID == id })
	if i < 0 {
		writeError(w, http.StatusNotFound, "no such VLAN")
		return
	}
	var in struct {
		Name   *string `json:"name"`
		Color  *string `json:"color"`
		Subnet *string `json:"subnet"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	v := catalog[i]
	if in.Name != nil {
		v.Name = *in.Name
	}
	if in.Color != nil {
		v.Color = *in.Color
	}
	if in.Subnet != nil {
		v.Subnet = *in.Subnet
	}
	s.saveVlan(w, r, v, http.StatusOK)
}

// saveVlan stores a VLAN and renames it on the devices that have it; a device
// refusing answers 502 with the changes made.
func (s *Server) saveVlan(w http.ResponseWriter, r *http.Request, v store.Vlan, status int) {
	ctx, cancel := changeContext(r)
	defer cancel()
	saved, done, err := s.Collector.SaveVlan(ctx, v)
	switch {
	case errors.Is(err, store.ErrInvalidVlan):
		writeError(w, http.StatusBadRequest, err.Error())
	case err != nil && done != nil:
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "changes": done})
	case err != nil:
		s.changeError(w, err)
	default:
		writeJSON(w, status, map[string]any{"vlan": saved, "changes": nonNil(done)})
	}
}

func (s *Server) deleteVlan(w http.ResponseWriter, r *http.Request) {
	id, ok := vlanID(w, r)
	if !ok {
		return
	}
	ctx, cancel := changeContext(r)
	defer cancel()
	done, err := s.Collector.DeleteVlan(ctx, id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "no such VLAN")
	case errors.Is(err, collector.ErrVlanOnDevices):
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "changes": nonNil(done)})
	case err != nil:
		s.changeError(w, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"changes": nonNil(done)})
	}
}

type portVlansInput struct {
	Port    string          `json:"port"`
	Value   model.PortVlans `json:"value"`
	Confirm bool            `json:"confirm"`
}

// planVlans is what putting a port in VLANs changes, device by device.
func (s *Server) planVlans(w http.ResponseWriter, r *http.Request) {
	var in portVlansInput
	if !readJSON(w, r, &in) {
		return
	}
	plan, err := s.Collector.PlanVlans(r.Context(), r.PathValue("id"), in.Port, in.Value)
	if err != nil {
		s.changeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// applyVlans applies the plan; a destructive step answers 409 like a change.
func (s *Server) applyVlans(w http.ResponseWriter, r *http.Request) {
	var in portVlansInput
	if !readJSON(w, r, &in) {
		return
	}
	ctx, cancel := changeContext(r)
	defer cancel()
	done, err := s.Collector.ApplyVlans(ctx, r.PathValue("id"), in.Port, in.Value, in.Confirm)
	if err != nil && len(done) > 0 {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "changes": done})
		return
	}
	if err != nil {
		s.changeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": done})
}

// changeError answers a change that was not made.
func (s *Server) changeError(w http.ResponseWriter, err error) {
	var de *collector.DestructiveError
	switch {
	case errors.As(err, &de):
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "destructive", "reason": de.Reason, "node": de.Node, "allowed": de.Allowed,
		})
	case errors.Is(err, collector.ErrNotChangeable):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, collector.ErrInvalidChange):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusBadGateway, err.Error())
	}
}

// changeContext is not cancelled with the request (see applyChange).
func changeContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), maxChangeTime)
}

func nonNil(list []store.ConfigChange) []store.ConfigChange {
	if list == nil {
		return []store.ConfigChange{}
	}
	return list
}
