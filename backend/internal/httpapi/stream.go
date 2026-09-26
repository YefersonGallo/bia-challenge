package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/yefersongallo/bia-energy/backend/internal/live"
)

// Heartbeat keeps proxies from closing an idle stream.
var Heartbeat = 15 * time.Second

func writeEvent(w http.ResponseWriter, id uint64, kind string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, kind, b)
	return err
}

// stream serves GET /api/stream as Server-Sent Events.
//
// A new connection receives a "snapshot" (clock, meters with their last readings and
// alerts) and then "tick", "alert", "control" and "snapshot" (after a reset) events. A
// reconnection with Last-Event-ID receives only what it missed, when it is still buffered.
func (s *server) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "streaming not supported")
		return
	}
	last, _ := strconv.ParseUint(r.Header.Get("Last-Event-ID"), 10, 64)
	if last == 0 {
		last, _ = strconv.ParseUint(r.URL.Query().Get("last_event_id"), 10, 64)
	}
	snap, snapID, missed, ch, cancel := s.cfg.Live.Subscribe(last)
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "retry: 2000\n\n")
	if snap != nil {
		if err := writeEvent(w, snapID, "snapshot", snap); err != nil {
			return
		}
	}
	for _, m := range missed {
		if err := writeEvent(w, m.ID, m.Kind, m.Data); err != nil {
			return
		}
	}
	fl.Flush()

	ping := time.NewTicker(Heartbeat)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case m, ok := <-ch:
			if !ok { // too slow: the client reconnects with its Last-Event-ID
				return
			}
			if err := writeEvent(w, m.ID, m.Kind, m.Data); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *server) streamState(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"state": s.cfg.Live.State(), "alerts": s.cfg.Live.Alerts()})
}

func (s *server) streamControl(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Action string  `json:"action"`
		Speed  float64 `json:"speed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID", "invalid JSON body")
		return
	}
	st, ok := s.cfg.Live.Control(body.Action, body.Speed)
	if !ok {
		writeError(w, http.StatusBadRequest, "INVALID", `action must be start|pause|reset|speed (speed between 0.25 and 32)`)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// Compile-time check that the runner satisfies the port.
var _ LiveStream = (*live.Runner)(nil)
