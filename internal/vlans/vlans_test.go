package vlans

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/riccardoalv/omini/internal/model"
	"github.com/riccardoalv/omini/internal/topology"
)

var all = []model.ChangeKind{model.ChangeKindPortVlans, model.ChangeKindVlanSet, model.ChangeKindVlanDelete}

func access(id uint16) *model.PortVlans { return &model.PortVlans{Untagged: model.Ptr(id)} }

func sw(id string, vlans []model.Vlan, ports ...string) topology.Node {
	d := &model.Device{Key: id, Vlans: vlans}
	for _, p := range ports {
		d.Interfaces = append(d.Interfaces, model.Interface{Name: p, Vlans: access(1)})
	}
	return topology.Node{ID: id, Kind: topology.KindDevice, Label: id, Role: "switch", Device: d, Changes: all}
}

// fw ── lan:Port 9 ── core ── Port 3:Port 9 ── edge ── Port 5 (the TV)
func network() topology.Topology {
	one := []model.Vlan{{ID: 1}}
	return topology.Topology{
		Nodes: []topology.Node{
			{ID: "fw", Kind: topology.KindDevice, Label: "fw", Role: "firewall", Device: &model.Device{Key: "fw", Vlans: one}},
			sw("core", one, "Port 3", "Port 9"),
			sw("edge", one, "Port 5", "Port 9"),
		},
		Edges: []topology.Edge{
			{ID: "1", Source: "fw", SourcePort: "lan", Target: "core", TargetPort: "Port 9", Kind: topology.EdgeLLDP},
			{ID: "2", Source: "core", SourcePort: "Port 3", Target: "edge", TargetPort: "Port 9", Kind: topology.EdgeFDB},
		},
	}
}

var catalog = []Entry{{ID: 1, Name: "default"}, {ID: 20, Name: "TV"}}

func describe(steps []Step) []string {
	var out []string
	for _, s := range steps {
		v, _ := json.Marshal(s.Value)
		parts := []string{s.NodeID, string(s.Kind)}
		if s.Port != "" {
			parts = append(parts, s.Port)
		}
		out = append(out, strings.Join(append(parts, string(v)), " "))
	}
	return out
}

func TestThePathToTheRouterCarriesTheVLAN(t *testing.T) {
	steps, notes, err := Plan(network(), catalog, "edge", "Port 5", *access(20))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`edge vlan.set {"id":20,"name":"TV"}`,
		`edge port.vlans Port 5 {"untagged":20}`,
		`edge port.vlans Port 9 {"tagged":[20],"untagged":1}`, // the edge's uplink
		`core vlan.set {"id":20,"name":"TV"}`,
		`core port.vlans Port 3 {"tagged":[20],"untagged":1}`, // down to the edge
		`core port.vlans Port 9 {"tagged":[20],"untagged":1}`, // up to the router
	}
	if got := describe(steps); !slices.Equal(got, want) {
		t.Fatalf("steps:\n%s", strings.Join(got, "\n"))
	}
	if len(notes) != 1 || notes[0].NodeID != "fw" || notes[0].Reason != "router" || !slices.Equal(notes[0].Vlans, []uint16{20}) {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestNothingToDoWhereTheVLANIsAlready(t *testing.T) {
	topo := network()
	for i := range topo.Nodes {
		topo.Nodes[i].Device.Vlans = append(topo.Nodes[i].Device.Vlans, model.Vlan{ID: 20})
		for j := range topo.Nodes[i].Device.Interfaces {
			if topo.Nodes[i].Device.Interfaces[j].Name != "Port 5" {
				topo.Nodes[i].Device.Interfaces[j].Vlans = &model.PortVlans{Untagged: model.Ptr[uint16](1), Tagged: []uint16{20}}
			}
		}
	}
	steps, notes, err := Plan(topo, catalog, "edge", "Port 5", *access(20))
	if err != nil || len(notes) != 0 {
		t.Fatal(err, notes)
	}
	if got := describe(steps); !slices.Equal(got, []string{`edge port.vlans Port 5 {"untagged":20}`}) {
		t.Fatalf("steps: %v", got)
	}
}

func TestADeviceOmininDoesNotManageIsANote(t *testing.T) {
	topo := network()
	topo.Nodes[1].Changes = nil // core: read only
	steps, notes, err := Plan(topo, catalog, "edge", "Port 5", *access(20))
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 || len(notes) != 1 || notes[0].NodeID != "core" || notes[0].Reason != "unmanaged" {
		t.Fatalf("steps %v notes %+v", describe(steps), notes)
	}
}

func TestPlanErrors(t *testing.T) {
	if _, _, err := Plan(network(), catalog, "edge", "Port 5", *access(30)); err == nil {
		t.Error("a VLAN outside the catalog")
	}
	if _, _, err := Plan(network(), catalog, "fw", "lan", *access(20)); err == nil {
		t.Error("a device that cannot change VLANs")
	}
	if _, _, err := Plan(network(), catalog, "edge", "Port 7", *access(20)); err == nil {
		t.Error("an unknown port")
	}
}

func TestImport(t *testing.T) {
	topo := network()
	topo.Nodes[1].Device.Vlans = []model.Vlan{{ID: 1}, {ID: 20, Name: model.Ptr("TV")}}
	topo.Nodes[2].Device.Vlans = []model.Vlan{{ID: 10, Name: model.Ptr("Office")}, {ID: 20, Subnet: model.Ptr("192.168.20.0/24")}}
	got := Import(topo)
	if len(got) != 3 || got[0].ID != 1 || got[1].Name != "Office" || got[2] != (Entry{ID: 20, Name: "TV", Subnet: "192.168.20.0/24"}) {
		t.Fatalf("import: %+v", got)
	}
}

// An inferred link names no port on the switch: the port where the switch
// learned the parent's MAC is its uplink.
func TestTheUplinkPortFromTheMACTable(t *testing.T) {
	topo := network()
	topo.Edges[1].TargetPort = ""
	topo.Nodes[1].MAC = "1c:2a:a3:00:00:01" // core
	topo.Nodes[2].Device.Fdb = []model.FdbEntry{{MAC: "1c:2a:a3:00:00:01", Port: "Port 9"}}
	steps, _, err := Plan(topo, catalog, "edge", "Port 5", *access(20))
	if err != nil {
		t.Fatal(err)
	}
	if got := describe(steps); !slices.Contains(got, `edge port.vlans Port 9 {"tagged":[20],"untagged":1}`) {
		t.Fatalf("steps: %v", got)
	}
}
