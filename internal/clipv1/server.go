// Package clipv1 provides the CLIP-v1 HTTP interface that the Ambilight TV
// expects: /description.xml, pairing (POST /api), config and (in later
// milestones) lights/groups as well as activating the entertainment stream.
package clipv1

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/trick77/relume/internal/config"
	"github.com/trick77/relume/internal/upnp"
)

// linkWindow is the duration during which a pairing is accepted after pressing
// the (virtual) link button — just like on a real bridge.
const linkWindow = 30 * time.Second

// LightProvider supplies the (already v1-translated) light list of the Bridge Pro
// and sets light states (REST fallback). It is set by the backend (M2+); if it is
// nil, the server returns empty lists (M1).
type LightProvider interface {
	// LightsV1 returns the v1 light list (key = numeric ID as a string).
	LightsV1() (map[string]any, error)
	// SetLightV1 sets the state of a light by its v1 ID.
	SetLightV1(v1id string, v1state map[string]any) error
}

// Server serves the CLIP-v1 interface.
type Server struct {
	cfg      *config.Config
	advIP    string
	httpPort int
	log      *slog.Logger
	lights   LightProvider
	// Debug enables verbose request logging (User-Agent + body) — helpful for
	// analyzing the real behavior of unknown TVs.
	Debug bool

	mu       sync.Mutex
	lastLink time.Time
}

// New creates the CLIP-v1 server.
func New(cfg *config.Config, advIP string, httpPort int, log *slog.Logger) *Server {
	return &Server{cfg: cfg, advIP: advIP, httpPort: httpPort, log: log}
}

// SetLightProvider registers the source for the light list (Bridge Pro backend).
func (s *Server) SetLightProvider(p LightProvider) {
	s.lights = p
}

// Handler returns the HTTP handler (routing) for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /description.xml", s.handleDescription)
	mux.HandleFunc("POST /api", s.handlePairing)
	mux.HandleFunc("POST /api/", s.handlePairing) // some clients append a trailing "/"
	mux.HandleFunc("GET /api/config", s.handleShortConfig)
	mux.HandleFunc("GET /config", s.handleShortConfig)
	mux.HandleFunc("GET /api/{user}/config", s.handleConfig)
	mux.HandleFunc("GET /api/{user}", s.handleDatastore)
	mux.HandleFunc("GET /api/{user}/lights", s.handleLights)
	mux.HandleFunc("PUT /api/{user}/lights/{id}/state", s.handleSetLightState)
	mux.HandleFunc("GET /api/{user}/groups", s.handleGroups)
	mux.HandleFunc("PUT /api/{user}/groups/{id}/action", s.handleGroupAction)
	mux.HandleFunc("PUT /api/{user}/groups/{id}", s.handleGroupUpdate)
	// Virtual link button (web UI / CLI open the pairing window).
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("POST /link", s.handleLink)
	return s.logRequests(mux)
}

// logRequests logs every request. In debug mode it also logs the User-Agent and
// body — essential for analyzing the real behavior of unknown TVs
// (e.g. the devicetype string during pairing).
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Debug {
			var body []byte
			if r.Body != nil {
				body, _ = io.ReadAll(io.LimitReader(r.Body, 4096))
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			s.log.Info("http rx",
				"method", r.Method,
				"path", r.URL.Path,
				"from", r.RemoteAddr,
				"user-agent", r.UserAgent(),
				"body", string(body),
			)
		} else {
			s.log.Info("http", "method", r.Method, "path", r.URL.Path, "from", r.RemoteAddr)
		}
		next.ServeHTTP(w, r)
	})
}

// PressLink opens the pairing window (used by the CLI `link` command or the web UI).
func (s *Server) PressLink() {
	s.mu.Lock()
	s.lastLink = time.Now()
	s.mu.Unlock()
	s.log.Info("link button pressed", "fenster", linkWindow)
}

func (s *Server) linkActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Since(s.lastLink) <= linkWindow
}

func (s *Server) handleDescription(w http.ResponseWriter, _ *http.Request) {
	xml, err := upnp.Render(s.cfg.Identity, s.advIP, s.httpPort)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	io.WriteString(w, xml)
}

type pairingRequest struct {
	DeviceType        string `json:"devicetype"`
	GenerateClientKey bool   `json:"generateclientkey"`
}

func (s *Server) handlePairing(w http.ResponseWriter, r *http.Request) {
	var req pairingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 2, "/", "body contains invalid json")
		return
	}
	s.log.Info("pairing request", "devicetype", req.DeviceType, "clientkey", req.GenerateClientKey)

	if !s.linkActive() {
		// CLIP-v1 standard error 101: link button not pressed.
		writeError(w, 101, "", "link button not pressed")
		return
	}

	username, err := randomHex(16) // 32 characters
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	user := &config.ApiUser{Username: username, DeviceType: req.DeviceType}

	success := map[string]string{"username": username}
	if req.GenerateClientKey {
		ck, cerr := randomHex(16)
		if cerr != nil {
			http.Error(w, cerr.Error(), http.StatusInternalServerError)
			return
		}
		ck = strings.ToUpper(ck)
		user.ClientKey = ck
		success["clientkey"] = ck
	}
	if err := s.cfg.AddApiUser(user); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.log.Info("tv paired", "username", username, "entertainment", req.GenerateClientKey)

	writeJSON(w, []map[string]any{{"success": success}})
}

// handleShortConfig returns the unauthenticated short config (identity check).
func (s *Server) handleShortConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.shortConfig())
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	writeJSON(w, s.shortConfig())
}

// shortConfig builds the config object; modelid MUST be BSB002.
func (s *Server) shortConfig() map[string]any {
	id := s.cfg.Identity
	return map[string]any{
		"name":             "Philips hue",
		"datastoreversion": "131",
		"swversion":        "1967054020",
		"apiversion":       "1.67.0",
		"mac":              id.MAC(),
		"bridgeid":         id.BridgeID(),
		"factorynew":       false,
		"replacesbridgeid": nil,
		"modelid":          "BSB002",
		"starterkitid":     "",
	}
}

// handleDatastore returns the top-level structure that some clients query after
// pairing.
func (s *Server) handleDatastore(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	writeJSON(w, map[string]any{
		"lights":        map[string]any{},
		"groups":        map[string]any{},
		"config":        s.shortConfig(),
		"schedules":     map[string]any{},
		"scenes":        map[string]any{},
		"rules":         map[string]any{},
		"sensors":       map[string]any{},
		"resourcelinks": map[string]any{},
	})
}

// handleLights returns the lights of the Bridge Pro (v1-translated) or an empty
// list if no backend is paired yet (M1).
func (s *Server) handleLights(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	if s.lights == nil {
		writeJSON(w, map[string]any{})
		return
	}
	lights, err := s.lights.LightsV1()
	if err != nil {
		s.log.Warn("reading lights from bridge pro", "err", err)
		writeJSON(w, map[string]any{})
		return
	}
	writeJSON(w, lights)
}

// handleSetLightState handles the REST control path: accept v1 state, translate
// it to v2 and forward it to the Bridge Pro.
func (s *Server) handleSetLightState(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	id := r.PathValue("id")
	var state map[string]any
	if err := json.NewDecoder(r.Body).Decode(&state); err != nil {
		writeError(w, 2, "/lights/"+id+"/state", "invalid json")
		return
	}
	if s.lights == nil {
		writeError(w, 3, "/lights/"+id, "no bridge pro paired")
		return
	}
	if err := s.lights.SetLightV1(id, state); err != nil {
		s.log.Warn("setting light", "id", id, "err", err)
		writeError(w, 901, "/lights/"+id+"/state", "bridge pro error")
		return
	}
	// v1 success response: one success entry per field that was set.
	resp := make([]map[string]any, 0, len(state))
	for k, v := range state {
		resp = append(resp, map[string]any{"success": map[string]any{
			"/lights/" + id + "/state/" + k: v,
		}})
	}
	writeJSON(w, resp)
}

func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	writeJSON(w, map[string]any{})
}

// handleGroupAction is the groups REST path. Full group/entertainment support
// follows in M4; for now the request is logged and acknowledged so that the TV
// does not abort.
func (s *Server) handleGroupAction(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	id := r.PathValue("id")
	body, _ := io.ReadAll(r.Body)
	s.log.Info("group action (not yet forwarded)", "group", id, "body", string(body))
	writeJSON(w, []map[string]any{{"success": map[string]any{"/groups/" + id + "/action": "ok"}}})
}

// handleGroupUpdate intercepts, among other things, the stream activation
// (PUT /groups/{id} with {"stream":{"active":true}}) — the entry into the
// entertainment path (M4).
func (s *Server) handleGroupUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	id := r.PathValue("id")
	body, _ := io.ReadAll(r.Body)
	s.log.Info("group update", "group", id, "body", string(body))
	writeJSON(w, []map[string]any{{"success": map[string]any{"/groups/" + id: "ok"}}})
}

// authorized checks whether the {user} from the path is a paired client.
func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	user := r.PathValue("user")
	if !s.cfg.HasApiUser(user) {
		writeError(w, 1, "/"+strings.TrimPrefix(r.URL.Path, "/api/"), "unauthorized user")
		return false
	}
	return true
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, indexHTML)
}

func (s *Server) handleLink(w http.ResponseWriter, _ *http.Request) {
	s.PressLink()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, fmt.Sprintf("<p>Link button pressed. Pairing open for %s.</p><p><a href=\"/\">back</a></p>", linkWindow))
}

const indexHTML = `<!doctype html><html><head><meta charset="utf-8"><title>relume</title></head>
<body style="font-family:sans-serif;max-width:40em;margin:2em auto">
<h1>relume</h1>
<p>Software bridge for Philips Ambilight TV &harr; Hue Bridge Pro.</p>
<form method="post" action="/link"><button type="submit">Press link button (open pairing)</button></form>
</body></html>`

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a CLIP-v1 error in the standard format.
func writeError(w http.ResponseWriter, typ int, address, description string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]map[string]any{{
		"error": map[string]any{"type": typ, "address": address, "description": description},
	}})
}
