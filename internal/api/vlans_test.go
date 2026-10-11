package api_test

import (
	"net/http"
	"net/url"
	"testing"
)

func TestVlanCatalog(t *testing.T) {
	h := newHarness(t, nil, fakeRouter(t))
	h.login()
	h.do("POST", "/api/integrations", map[string]any{"type": "fakert", "config": map[string]any{"host": "192.168.1.1"}}, nil)
	if err := h.coll.CollectNow(t.Context()); err != nil {
		t.Fatal(err)
	}
	var list []map[string]any
	if h.do("GET", "/api/vlans", nil, &list); len(list) != 0 {
		t.Fatalf("nothing to import: %v", list)
	}
	if code := h.do("POST", "/api/vlans", map[string]any{"id": 20, "name": "Guests", "subnet": "192.168.20.0/24"}, nil); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	if code := h.do("POST", "/api/vlans", map[string]any{"id": 20, "name": "Again"}, nil); code != http.StatusConflict {
		t.Fatalf("twice: %d", code)
	}
	if code := h.do("POST", "/api/vlans", map[string]any{"id": 30, "subnet": "192.168.30.1/24"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a subnet that is not a network: %d", code)
	}
	if code := h.do("PATCH", "/api/vlans/20", map[string]any{"name": "Visitors", "color": "green"}, nil); code != http.StatusOK {
		t.Fatalf("rename: %d", code)
	}
	h.do("GET", "/api/vlans", nil, &list)
	if len(list) != 1 || list[0]["name"] != "Visitors" || list[0]["color"] != "green" || list[0]["devices"] == nil {
		t.Fatalf("list: %v", list)
	}
	if code := h.do("DELETE", "/api/vlans/20", nil, nil); code != http.StatusOK {
		t.Fatalf("delete: %d", code)
	}
	if code := h.do("DELETE", "/api/vlans/20", nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete again: %d", code)
	}

	// The firewall here cannot change ports' VLANs.
	fw := "/api/nodes/" + url.PathEscape("dev:58:9c:fc:00:00:01")
	if code := h.do("POST", fw+"/vlan-plan", map[string]any{"port": "lan", "value": map[string]any{"untagged": 1}}, nil); code != http.StatusBadRequest {
		t.Fatalf("plan: %d", code)
	}
}
