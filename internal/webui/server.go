package webui

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

//go:embed assets/*
var assetsFS embed.FS

// Server is the web UI HTTP server. It is separate from the TV-facing clipv1
// server and runs by default (suppressed by -headless).
type Server struct {
	addr         string
	hub          *Hub
	src          StateSource
	log          *slog.Logger
	http         *http.Server
	snapInterval time.Duration
}

// NewServer builds the UI server.
func NewServer(addr string, hub *Hub, src StateSource, log *slog.Logger) *Server {
	return &Server{addr: addr, hub: hub, src: src, log: log, snapInterval: time.Second}
}

// runSnapshotLoop periodically publishes a fresh snapshot to the hub so connected
// dashboards update live (lights, mode, stream health, pairing state). Without
// this, an SSE client only ever sees the single snapshot built at connect time
// plus log events. The cadence is coarse on purpose — this is a status view, not
// a frame mirror.
func (s *Server) runSnapshotLoop(ctx context.Context) {
	interval := s.snapInterval
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			// Skip when no browser is connected: BuildSnapshot reads the lights from
			// the Hue Bridge Pro (behind a cache), and the queue-sensitive Pro should not
			// be polled for an audience of nobody.
			if s.hub.hasSubscribers() {
				s.hub.SetSnapshot(BuildSnapshot(s.src))
			}
		}
	}
}

// Handler returns the routed HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/events", s.handleEvents)

	sub, _ := fs.Sub(assetsFS, "assets")
	mux.Handle("/", http.FileServer(http.FS(sub)))
	return mux
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(BuildSnapshot(s.src))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	write := func(b []byte) error {
		if _, err := w.Write(b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	writeFrame := func(f Frame) error {
		b, _ := json.Marshal(f)
		out := append([]byte("data: "), b...)
		return write(append(out, '\n', '\n'))
	}

	// Initial paint: a fresh snapshot, then the buffered event tail. Subscribe AFTER the
	// tail replay: subscribing first would buffer events published during the replay
	// window onto the channel AND include them in the tail, delivering them twice (the
	// ring carries up to 200 events — too many to seed through the 32-deep channel). The
	// inverse gap (an event slipping through the window) is a pre-existing, cosmetic
	// live-log nicety, not worth a duplicate on every connect.
	snap := BuildSnapshot(s.src)
	if writeFrame(Frame{Kind: "snapshot", Snapshot: &snap}) != nil {
		return
	}
	for _, e := range s.hub.Events() {
		e := e
		if writeFrame(Frame{Kind: "event", Event: &e}) != nil {
			return
		}
	}

	ch, cancel := s.hub.Subscribe()
	defer cancel()

	// A periodic heartbeat (an SSE comment) keeps intermediaries from idle-closing the
	// stream AND surfaces a dead client: a write to a vanished peer eventually errors, so
	// the handler returns, cancel() runs, and the snapshot loop stops polling the
	// queue-sensitive Pro for an audience that is no longer there.
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if write([]byte(": ping\n\n")) != nil {
				return
			}
		case f, ok := <-ch:
			if !ok {
				return
			}
			if writeFrame(f) != nil {
				return
			}
		}
	}
}

// Run serves until ctx is cancelled. It returns a non-nil error only on a real
// bind/serve failure (never http.ErrServerClosed), so the caller can log it
// without taking down the headless service.
func (s *Server) Run(ctx context.Context) error {
	s.http = &http.Server{Addr: s.addr, Handler: s.Handler()}
	go s.runSnapshotLoop(ctx)
	errc := make(chan error, 1)
	go func() {
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errc <- err
		}
	}()
	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutCtx)
		return nil
	case err := <-errc:
		return err
	}
}
