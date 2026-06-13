// Package mdns actively announces relume as a Hue bridge via mDNS/Bonjour
// (_hue._tcp.local.). Modern Philips TVs (and the Bridge Pro itself) find the
// bridge primarily this way; they passively listen for the announcement and
// often make no request of their own. The format follows hass-emulated-hue,
// which the Ambilight TV is known to discover: instance name
// "Philips Hue - XXXXXX", TXT with bridgeid and modelid=BSB002.
package mdns

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/grandcat/zeroconf"
	"github.com/trick77/relume/internal/config"
)

const (
	service = "_hue._tcp"
	domain  = "local."
)

// Announcer keeps the mDNS registration alive.
type Announcer struct {
	id    config.Identity
	advIP string
	port  int
	log   *slog.Logger
}

// New creates an Announcer. port is the advertised SRV port (usually the
// HTTP port of the emulated bridge).
func New(id config.Identity, advIP string, port int, log *slog.Logger) *Announcer {
	return &Announcer{id: id, advIP: advIP, port: port, log: log}
}

// Run registers the service and keeps it alive until ctx is cancelled.
func (a *Announcer) Run(ctx context.Context) error {
	bridgeID := a.id.BridgeID()
	instance := "Philips Hue - " + bridgeID[len(bridgeID)-6:]
	txt := []string{
		"bridgeid=" + bridgeID,
		"modelid=BSB002",
	}

	var ifaces []net.Interface
	if iface, err := interfaceForIP(a.advIP); err != nil {
		a.log.Warn("mdns: interface for advertise IP not found, using all", "err", err)
	} else {
		ifaces = []net.Interface{*iface}
	}

	server, err := zeroconf.Register(instance, service, domain, a.port, txt, ifaces)
	if err != nil {
		return fmt.Errorf("mdns register: %w", err)
	}
	defer server.Shutdown()

	a.log.Info("mdns: announced as hue bridge",
		"instance", instance, "service", service, "port", a.port, "bridgeid", bridgeID)

	<-ctx.Done()
	return ctx.Err()
}

// interfaceForIP returns the multicast-capable interface that carries the given IP.
func interfaceForIP(ip string) (*net.Interface, error) {
	target := net.ParseIP(ip)
	if target == nil {
		return nil, fmt.Errorf("invalid IP %q", ip)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for i := range ifaces {
		if ifaces[i].Flags&net.FlagMulticast == 0 || ifaces[i].Flags&net.FlagUp == 0 {
			continue
		}
		addrs, aerr := ifaces[i].Addrs()
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(target) {
				return &ifaces[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no multicast-capable interface with IP %s", ip)
}
