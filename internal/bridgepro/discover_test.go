package bridgepro

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestDiscover_rateLimitedReturnsRateLimitedError(t *testing.T) {
	// Discover's URL is hardcoded to the real cloud, so exercise the Retry-After parsing
	// and the error type (the two pieces of the 429 handling) directly.
	if got := parseRetryAfter("54"); got != 54*time.Second {
		t.Fatalf("parseRetryAfter(54) = %s, want 54s", got)
	}
	if got := parseRetryAfter(""); got != 60*time.Second {
		t.Fatalf("parseRetryAfter(empty) = %s, want 60s fallback", got)
	}
	if got := parseRetryAfter("99999"); got != 5*time.Minute {
		t.Fatalf("parseRetryAfter(huge) = %s, want 5m ceiling", got)
	}
	var rle = &RateLimitedError{RetryAfter: 54 * time.Second}
	if !errors.Is(rle, ErrCloudRateLimited) {
		t.Fatal("RateLimitedError must satisfy errors.Is(ErrCloudRateLimited)")
	}
}
