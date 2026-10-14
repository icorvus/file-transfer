package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"neardrop/internal/discovery"
	"neardrop/internal/receiver"
)

func main() {
	if err := run(); err != nil {
		slog.Error("NearDrop stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	hostname, _ := os.Hostname()
	name := flag.String("name", hostname, "Name shown on iPhone")
	out := flag.String("out", filepath.Join(home, "Pictures", "NearDrop"), "Folder for incoming photos")
	state := flag.String("state", filepath.Join(config, "neardrop"), "Private identity and pairing directory")
	port := flag.Int("port", 8787, "HTTPS transfer port (local network)")
	admin := flag.Int("admin-port", 8788, "Control page port (loopback only)")
	maxMB := flag.Int64("max-mb", 2048, "Maximum size of each file in MiB")
	noMDNS := flag.Bool("no-mdns", false, "Disable Bonjour advertisement")
	flag.Parse()
	if *port < 1 || *port > 65535 || *admin < 1 || *admin > 65535 || *port == *admin {
		return fmt.Errorf("choose two different ports between 1 and 65535")
	}
	if *maxMB < 1 || *maxMB > 1048576 {
		return fmt.Errorf("max-mb must be between 1 and 1048576")
	}
	if err := os.MkdirAll(*state, 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(*state, "receiver.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("another receiver is already using this state directory: %w", err)
	}
	s, err := receiver.New(receiver.Config{Name: *name, OutputDir: *out, StateDir: *state, Port: *port, AdminPort: *admin, MaxBytes: *maxMB << 20})
	if err != nil {
		return err
	}
	lan, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		return err
	}
	defer lan.Close()
	local, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *admin))
	if err != nil {
		return err
	}
	defer local.Close()
	transfer := &http.Server{Handler: s.TransferHandler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Minute, WriteTimeout: 30 * time.Minute, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{s.Certificate}}}
	dashboard := &http.Server{Handler: s.AdminHandler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	if !*noMDNS {
		advertisement, err := discovery.Publish(*name, s.ID(), *port)
		if err != nil {
			slog.Warn("Bonjour unavailable; QR address still works", "error", err)
		} else {
			defer advertisement.Shutdown()
		}
	}
	fmt.Printf("\nNearDrop is ready\n\n  Control page: http://127.0.0.1:%d\n  Saving to:    %s\n  LAN port:     %d (HTTPS)\n\nOpen the control page to pair your iPhone.\n\n", *admin, s.Config.OutputDir, *port)
	errors := make(chan error, 2)
	go func() { errors <- transfer.ServeTLS(lan, "", "") }()
	go func() { errors <- dashboard.Serve(local) }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case <-ctx.Done():
	case err = <-errors:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = dashboard.Shutdown(shutdown)
	_ = transfer.Shutdown(shutdown)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
