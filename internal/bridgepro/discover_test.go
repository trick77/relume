package bridgepro

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// proHost strips the scheme from a TLS test server URL, yielding the host:port that
// FetchModelID expects (it prepends https:// itself).
func proHost(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return strings.TrimPrefix(srv.URL, "https://")
}

func TestFetchModelID_ParsesBSB003(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/0/config" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"name":"Hue","modelid":"BSB003","swversion":"x"}`))
	}))
	defer srv.Close()

	got, err := FetchModelID(proHost(t, srv))
	if err != nil {
		t.Fatalf("FetchModelID: %v", err)
	}
	if got != ModelHueBridgePro {
		t.Fatalf("modelid = %q, want %q", got, ModelHueBridgePro)
	}
}

func TestFetchModelID_ReturnsNonProModel(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"modelid":"BSB002"}`))
	}))
	defer srv.Close()

	got, err := FetchModelID(proHost(t, srv))
	if err != nil {
		t.Fatalf("FetchModelID: %v", err)
	}
	if got != "BSB002" {
		t.Fatalf("modelid = %q, want BSB002 (a non-Pro must be reported as-is, not coerced)", got)
	}
	if got == ModelHueBridgePro {
		t.Fatal("a BSB002 bridge must not be classified as a Pro")
	}
}

func TestFetchModelID_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := FetchModelID(proHost(t, srv)); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

// sanity: the test TLS server uses a self-signed cert, so the call only works because
// FetchModelID skips verification (as documented). Guard against a regression that
// re-enables verification here.
func TestFetchModelID_SkipsTLSVerification(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"modelid":"BSB003"}`))
	}))
	srv.TLS = &tls.Config{} // default self-signed
	srv.StartTLS()
	defer srv.Close()

	if _, err := FetchModelID(proHost(t, srv)); err != nil {
		t.Fatalf("FetchModelID against a self-signed cert: %v", err)
	}
}

func TestParseHueTXT(t *testing.T) {
	bid, mid := parseHueTXT([]string{"bridgeid=001788FFFEAABBCC", "modelid=BSB003", "other=x"})
	if bid != "001788FFFEAABBCC" {
		t.Errorf("bridgeid = %q", bid)
	}
	if mid != "BSB003" {
		t.Errorf("modelid = %q", mid)
	}
	// Missing keys yield empty strings, malformed entries are ignored.
	bid, mid = parseHueTXT([]string{"novalue", ""})
	if bid != "" || mid != "" {
		t.Errorf("expected empty, got bridgeid=%q modelid=%q", bid, mid)
	}
}

func TestFirstIPv4(t *testing.T) {
	if got := firstIPv4([]net.IP{net.ParseIP("fe80::1"), net.ParseIP("192.168.1.5")}); got != "192.168.1.5" {
		t.Errorf("firstIPv4 = %q, want 192.168.1.5", got)
	}
	if got := firstIPv4([]net.IP{net.ParseIP("fe80::1")}); got != "" {
		t.Errorf("firstIPv4 (no v4) = %q, want empty", got)
	}
}
