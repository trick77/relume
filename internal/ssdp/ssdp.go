// Package ssdp implementiert den SSDP/UPnP-Responder, mit dem sich ambibridge
// gegenüber dem TV als Gen-2-Hue-Bridge zu erkennen gibt. Es lauscht auf dem
// Multicast 239.255.255.250:1900, beantwortet M-SEARCH-Queries sofort und sendet
// periodische NOTIFY ssdp:alive-Broadcasts.
package ssdp

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/trick77/ambibridge/internal/config"
)

const (
	multicastAddr = "239.255.255.250:1900"
	// server ist der exakte SERVER-Header einer echten Hue-Bridge (verifiziert via diyHue).
	server      = "Linux/3.14.0 UPnP/1.0 IpBridge/1.20.0"
	notifyEvery = 60 * time.Second
)

// Responder beantwortet SSDP-Anfragen für eine Bridge-Identität.
type Responder struct {
	id       config.Identity
	advIP    string // beworbene IP im LOCATION-Header
	httpPort int
	log      *slog.Logger
}

// New erstellt einen Responder. advIP ist die IP, die im LOCATION-Header beworben
// wird (die Adresse des HTTP-Servers von ambibridge).
func New(id config.Identity, advIP string, httpPort int, log *slog.Logger) *Responder {
	return &Responder{id: id, advIP: advIP, httpPort: httpPort, log: log}
}

// Run startet Listener und periodische NOTIFYs und blockiert bis ctx beendet wird.
func (r *Responder) Run(ctx context.Context) error {
	group := &net.UDPAddr{IP: net.ParseIP("239.255.255.250"), Port: 1900}
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return fmt.Errorf("ssdp multicast listen: %w", err)
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(1 << 20)

	go r.notifyLoop(ctx, conn, group)

	r.log.Info("ssdp responder gestartet", "advertise", r.advIP, "httpPort", r.httpPort)

	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			r.log.Warn("ssdp read", "err", err)
			continue
		}
		r.handle(conn, src, buf[:n])
	}
}

// handle beantwortet M-SEARCH-Queries; alles andere wird ignoriert.
func (r *Responder) handle(conn *net.UDPConn, src *net.UDPAddr, data []byte) {
	msg := string(data)
	if !strings.HasPrefix(msg, "M-SEARCH") {
		return
	}
	// Wir antworten breit (ohne strenges ST-Matching), wie es echte Bridges tun —
	// der TV filtert per LOCATION/description.xml. Sofortige Antwort ist wichtig,
	// da der TV ein kurzes Suchfenster hat (diyHue #988).
	for _, resp := range r.searchResponses() {
		if _, err := conn.WriteToUDP([]byte(resp), src); err != nil {
			r.log.Warn("ssdp respond", "err", err, "to", src.String())
			return
		}
	}
}

// searchResponses liefert die drei M-SEARCH-200-OK-Antworten (root, uuid, basic).
func (r *Responder) searchResponses() []string {
	uuid := r.id.UUID()
	variants := []struct{ st, usn string }{
		{"upnp:rootdevice", "uuid:" + uuid + "::upnp:rootdevice"},
		{"uuid:" + uuid, "uuid:" + uuid},
		{"urn:schemas-upnp-org:device:basic:1", "uuid:" + uuid},
	}
	out := make([]string, 0, len(variants))
	for _, v := range variants {
		out = append(out, "HTTP/1.1 200 OK\r\n"+
			"HOST: 239.255.255.250:1900\r\n"+
			"EXT:\r\n"+
			"CACHE-CONTROL: max-age=100\r\n"+
			fmt.Sprintf("LOCATION: http://%s:%d/description.xml\r\n", r.advIP, r.httpPort)+
			"SERVER: "+server+"\r\n"+
			"hue-bridgeid: "+r.id.BridgeID()+"\r\n"+
			"ST: "+v.st+"\r\n"+
			"USN: "+v.usn+"\r\n"+
			"\r\n")
	}
	return out
}

// notifyLoop sendet periodisch NOTIFY ssdp:alive an den Multicast.
func (r *Responder) notifyLoop(ctx context.Context, conn *net.UDPConn, group *net.UDPAddr) {
	t := time.NewTicker(notifyEvery)
	defer t.Stop()
	r.sendNotify(conn, group)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.sendNotify(conn, group)
		}
	}
}

func (r *Responder) sendNotify(conn *net.UDPConn, group *net.UDPAddr) {
	uuid := r.id.UUID()
	variants := []struct{ nt, usn string }{
		{"upnp:rootdevice", "uuid:" + uuid + "::upnp:rootdevice"},
		{"uuid:" + uuid, "uuid:" + uuid},
		{"urn:schemas-upnp-org:device:basic:1", "uuid:" + uuid},
	}
	for _, v := range variants {
		msg := "NOTIFY * HTTP/1.1\r\n" +
			"HOST: 239.255.255.250:1900\r\n" +
			"CACHE-CONTROL: max-age=100\r\n" +
			fmt.Sprintf("LOCATION: http://%s:%d/description.xml\r\n", r.advIP, r.httpPort) +
			"SERVER: " + server + "\r\n" +
			"NTS: ssdp:alive\r\n" +
			"hue-bridgeid: " + r.id.BridgeID() + "\r\n" +
			"NT: " + v.nt + "\r\n" +
			"USN: " + v.usn + "\r\n" +
			"\r\n"
		if _, err := conn.WriteToUDP([]byte(msg), group); err != nil {
			r.log.Warn("ssdp notify", "err", err)
			return
		}
	}
}
