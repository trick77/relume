// Command ambibridge ist eine Software-Bridge, die einen Philips Ambilight-TV mit
// einer Hue Bridge Pro verbindet, indem sie sich gegenüber dem TV als Gen-2-Bridge
// ausgibt und Befehle an die Pro weiterreicht.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/trick77/ambibridge/internal/clipv1"
	"github.com/trick77/ambibridge/internal/config"
	"github.com/trick77/ambibridge/internal/ssdp"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		if err := runServe(os.Args[2:], log); err != nil {
			log.Error("serve beendet", "err", err)
			os.Exit(1)
		}
	case "link":
		if err := runLink(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unbekannter Befehl %q\nVerfügbar: serve, link\n", cmd)
		os.Exit(2)
	}
}

func runServe(args []string, log *slog.Logger) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	cfgPath := fs.String("config", "ambibridge.json", "Pfad zur Konfigurationsdatei")
	httpPort := fs.Int("http-port", 80, "HTTP-Port der emulierten Bridge")
	advIP := fs.String("advertise-ip", "", "beworbene IP (leer = auto-detektieren)")
	_ = fs.Parse(args)

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	ip := *advIP
	if ip == "" {
		ip, err = outboundIP()
		if err != nil {
			return fmt.Errorf("advertise-ip auto-detektieren: %w (nutze -advertise-ip)", err)
		}
	}
	log.Info("identität", "serial", cfg.Identity.Serial, "bridgeid", cfg.Identity.BridgeID(), "advertise", ip)

	clip := clipv1.New(cfg, ip, *httpPort, log)
	responder := ssdp.New(cfg.Identity, ip, *httpPort, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *httpPort),
		Handler: clip.Handler(),
	}

	errc := make(chan error, 2)
	go func() {
		log.Info("http-server gestartet", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errc <- fmt.Errorf("http: %w", err)
		}
	}()
	go func() {
		if err := responder.Run(ctx); err != nil && ctx.Err() == nil {
			errc <- fmt.Errorf("ssdp: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal")
	case err := <-errc:
		stop()
		shutdownHTTP(httpSrv)
		return err
	}
	shutdownHTTP(httpSrv)
	return nil
}

func shutdownHTTP(srv *http.Server) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// runLink öffnet das Pairing-Fenster, indem es den laufenden serve-Prozess anstößt.
func runLink(args []string) error {
	fs := flag.NewFlagSet("link", flag.ExitOnError)
	host := fs.String("host", "127.0.0.1", "Host des laufenden ambibridge")
	port := fs.Int("http-port", 80, "HTTP-Port")
	_ = fs.Parse(args)

	url := fmt.Sprintf("http://%s:%d/link", *host, *port)
	resp, err := http.Post(url, "application/x-www-form-urlencoded", nil)
	if err != nil {
		return fmt.Errorf("link-anfrage an %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("link-anfrage fehlgeschlagen: %s", resp.Status)
	}
	fmt.Println("Link-Button gedrückt – Pairing für 30s offen.")
	return nil
}

// outboundIP ermittelt die lokale IPv4, über die ausgehender Verkehr läuft.
func outboundIP() (string, error) {
	conn, err := net.Dial("udp4", "192.0.2.1:9") // TEST-NET-1, kein echter Traffic
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}
