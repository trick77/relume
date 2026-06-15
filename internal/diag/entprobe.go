package diag

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// EntertainmentProbe is a passive UDP listener on port 2100 — the port a Hue
// bridge uses for the Entertainment DTLS stream. relume does not (yet) implement
// DTLS, so this only OBSERVES: it answers the question "after the TV activates
// the entertainment stream, does it actually try to open a DTLS connection?".
//
// It never responds, so the TV's handshake still fails and it falls back to the
// REST control path exactly as in production — the listener just makes the
// attempt visible. A first datagram on :2100 proves the TV wants to stream over
// DTLS (so implementing M4 would fix the Ambilight lag); continued silence after
// a confirmed stream activation proves the TV will not use DTLS at all.
type EntertainmentProbe struct {
	bindIP string
	log    *slog.Logger

	mu          sync.Mutex
	packets     uint64
	bytes       uint64
	lastSrc     string
	firstLogged bool
}

// NewEntertainmentProbe creates the probe. bindIP is the advertised IP; binding
// to it (rather than 0.0.0.0) pins the listener to the interface that faces the
// TV on a multi-homed host (see AGENTS.md multi-NIC note). Empty binds all.
func NewEntertainmentProbe(bindIP string, log *slog.Logger) *EntertainmentProbe {
	return &EntertainmentProbe{bindIP: bindIP, log: log}
}

// Run binds udp :2100 and logs entertainment-stream attempts until ctx is done.
func (p *EntertainmentProbe) Run(ctx context.Context) error {
	addr := &net.UDPAddr{IP: net.ParseIP(p.bindIP), Port: 2100}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("entertainment probe listen on %s: %w", addr, err)
	}
	defer conn.Close()
	p.log.Info("entertainment dtls probe started (listening on udp :2100)", "bind", addr.String())

	go p.reportLoop(ctx)
	go func() {
		<-ctx.Done()
		_ = conn.Close() // unblock ReadFromUDP
	}()

	buf := make([]byte, 2048)
	for {
		n, src, rerr := conn.ReadFromUDP(buf)
		if rerr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		p.record(src, buf[:n])
	}
}

// record accounts one datagram and logs the very first one (with a DTLS kind
// classification) immediately — that first packet is the headline result.
func (p *EntertainmentProbe) record(src *net.UDPAddr, data []byte) {
	p.mu.Lock()
	p.packets++
	p.bytes += uint64(len(data))
	p.lastSrc = src.IP.String()
	first := !p.firstLogged
	p.firstLogged = true
	p.mu.Unlock()

	if first {
		p.log.Info("ENTERTAINMENT dtls probe: TV opened a stream on udp :2100",
			"from", src.IP.String(),
			"bytes", len(data),
			"kind", dtlsKind(data),
		)
	}
}

// reportLoop emits a rollup of stream traffic every 10s while there is activity.
func (p *EntertainmentProbe) reportLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	var prev uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.mu.Lock()
			packets, bytes, src := p.packets, p.bytes, p.lastSrc
			p.mu.Unlock()
			if packets == prev {
				continue
			}
			p.log.Info("ENTERTAINMENT dtls probe activity",
				"packets_total", packets,
				"bytes_total", bytes,
				"since_last", packets-prev,
				"from", src,
			)
			prev = packets
		}
	}
}

// dtlsKind classifies the first bytes of a UDP datagram as a (D)TLS record. A
// DTLS 1.2 record starts with content-type 0x16 (handshake) and version 0xfefd.
func dtlsKind(b []byte) string {
	if len(b) < 3 {
		return "too short"
	}
	if b[0] == 0x16 && b[1] == 0xfe && (b[2] == 0xfd || b[2] == 0xff) {
		return "DTLS handshake (ClientHello — TV wants entertainment streaming)"
	}
	if b[0] == 0x16 {
		return "TLS/DTLS handshake record"
	}
	return fmt.Sprintf("non-handshake datagram (first byte 0x%02x)", b[0])
}
