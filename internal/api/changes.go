package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/riccardoalv/omini/internal/collector"
	"github.com/riccardoalv/omini/internal/model"
)

// maxChangeTime bounds a change: waiting for a round in progress, then the
// plugin (at most 300 s).
const maxChangeTime = 10 * time.Minute

// applyChange applies a configuration change to a node's device. A
// destructive one answers 409 with why (reason, node) and whether it may be
// confirmed a second time (allowed); the device refusing it answers 502 with
// the change as kept in the history.
func (s *Server) applyChange(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind    model.ChangeKind `json:"kind"`
		Port    string           `json:"port"`
		Value   any              `json:"value"`
		Confirm bool             `json:"confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	// Not cancelled with the request: a closed tab must never stop a change
	// between applying and saving it (the plugin has its own time limit).
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), maxChangeTime)
	defer cancel()
	rec, err := s.Collector.Apply(ctx, collector.ChangeRequest{
		NodeID: r.PathValue("id"), Kind: in.Kind, Port: in.Port, Value: in.Value, Confirmed: in.Confirm,
	})
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
	case err != nil && rec.ID != 0:
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "change": rec})
	case err != nil:
		internalError(w, err)
	default:
		writeJSON(w, http.StatusOK, rec)
	}
}

// configChanges lists the latest changes made to a node.
func (s *Server) configChanges(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ConfigChanges(r.Context(), r.PathValue("id"), 50)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) changeSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"allow_destructive": s.Collector.AllowDestructive(r.Context())})
}

func (s *Server) setChangeSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		AllowDestructive bool `json:"allow_destructive"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.Collector.SetAllowDestructive(r.Context(), in.AllowDestructive); err != nil {
		internalError(w, err)
		return
	}
	s.changeSettings(w, r)
}
