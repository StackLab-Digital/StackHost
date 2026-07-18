package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"
)

type ingressServers struct {
	httpServer  *http.Server
	httpsServer *http.Server
}

func startIngressServers(a *app) *ingressServers {
	if getenv("STACKHOST_INGRESS_ENABLED", "false") != "true" || a.ingressProxy == nil {
		return nil
	}
	storage := getenv("STACKHOST_CERT_STORAGE", filepath.Join(getenv("STACKHOST_DATA_DIR", "./data"), "certificates"))
	_ = os.MkdirAll(storage, 0750)
	if a.docker != nil {
		mode := "standalone"
		_ = a.db.QueryRow(`SELECT runtime_mode FROM environment_settings WHERE id=1`).Scan(&mode)
		_ = a.docker.EnsureIngressNetwork(context.Background(), mode == "swarm")
	}
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Email:      os.Getenv("STACKHOST_ACME_EMAIL"),
		Cache:      autocert.DirCache(storage),
		HostPolicy: a.ingressHostPolicy,
	}
	proxyHandler := http.HandlerFunc(a.ingressProxy.Handler)
	httpServer := &http.Server{Addr: getenv("STACKHOST_INGRESS_HTTP_ADDR", ":80"), Handler: manager.HTTPHandler(proxyHandler), ReadHeaderTimeout: 10 * time.Second}
	httpsServer := &http.Server{Addr: getenv("STACKHOST_INGRESS_HTTPS_ADDR", ":443"), Handler: proxyHandler, ReadHeaderTimeout: 10 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: manager.GetCertificate}}
	servers := &ingressServers{httpServer: httpServer, httpsServer: httpsServer}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("ingress HTTP stopped", "error", err)
		}
	}()
	go func() {
		if err := httpsServer.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("ingress HTTPS stopped", "error", err)
		}
	}()
	return servers
}

func (s *ingressServers) Close(ctx context.Context) {
	if s == nil {
		return
	}
	_ = s.httpServer.Shutdown(ctx)
	_ = s.httpsServer.Shutdown(ctx)
}

func (a *app) ingressHostPolicy(ctx context.Context, hostname string) error {
	hostname = strings.ToLower(strings.TrimSpace(hostname))
	items, err := a.domains.store.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Hostname == hostname && item.HTTPSEnabled {
			return nil
		}
	}
	return errors.New("domain is not registered for HTTPS")
}
