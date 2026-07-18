package ingress

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateHostnameRejectsInternalAndURLValues(t *testing.T) {
	for _, value := range []string{"localhost", "127.0.0.1", "app.local", "https://example.com", "*.example.com", "example.com:443"} {
		if _, err := ValidateHostname(value); err == nil {
			t.Fatalf("hostname %q was accepted", value)
		}
	}
	hostname, err := ValidateHostname(" App.Example.COM ")
	if err != nil || hostname != "app.example.com" {
		t.Fatalf("hostname = %q, err=%v", hostname, err)
	}
}

func TestProxyRoutesOnlyRegisteredHostAndControlsForwardedHeaders(t *testing.T) {
	proxy := NewProxy(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("X-Forwarded-Host"); got != "app.example.com" {
			t.Errorf("forwarded host = %q", got)
		}
		if got := r.Header.Get("X-Forwarded-Proto"); got != "http" {
			t.Errorf("forwarded proto = %q", got)
		}
		if got := r.Header.Get("X-Real-IP"); got != "192.0.2.10" {
			t.Errorf("real ip = %q", got)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
	}))
	proxy.SetRoutes(context.Background(), []Domain{{Hostname: "app.example.com", Enabled: true, HTTPSEnabled: false}}, StaticResolver{"app.example.com": "http://target.internal"})
	request := httptest.NewRequest(http.MethodGet, "http://app.example.com/stream", nil)
	request.Host = "app.example.com"
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-Host", "attacker.example")
	response := httptest.NewRecorder()
	proxy.Handler(response, request)
	if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != "ok" {
		t.Fatalf("proxy response = %d %q", response.Code, response.Body.String())
	}
	unknown := httptest.NewRecorder()
	proxy.Handler(unknown, httptest.NewRequest(http.MethodGet, "http://unknown.example/", nil))
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown domain status = %d", unknown.Code)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
