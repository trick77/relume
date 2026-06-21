package diag

import (
	"net"
	"testing"
	"time"
)

func TestShouldLogMDNS_keepsDefaultHueOnly(t *testing.T) {
	// Given
	o := NewMDNSObserver("192.0.2.10", nil)

	// Then
	if !o.shouldLogMDNS(net.ParseIP("192.0.2.20"), []string{"_hue._tcp.local"}) {
		t.Fatal("expected hue query to be logged")
	}
	if o.shouldLogMDNS(net.ParseIP("192.0.2.20"), []string{"_googlecast._tcp.local"}) {
		t.Fatal("expected non-hue query to be ignored by default")
	}
}

func TestShouldLogMDNS_logsAllQuestionsFromTVIP(t *testing.T) {
	// Given
	o := NewMDNSObserver("192.0.2.10", nil)
	o.DebugTVIP = "192.0.2.30"

	// Then
	if !o.shouldLogMDNS(net.ParseIP("192.0.2.30"), []string{"_googlecast._tcp.local"}) {
		t.Fatal("expected all questions from the configured TV IP to be logged")
	}
	if o.shouldLogMDNS(net.ParseIP("192.0.2.31"), []string{"_googlecast._tcp.local"}) {
		t.Fatal("expected non-hue query from another host to be ignored")
	}
}

func TestReadName_terminatesOnPointerLoop(t *testing.T) {
	// A crafted DNS message whose two compression pointers reference each other: the
	// offset-12 pointer points at offset 14 and the offset-14 pointer back at 12. A
	// naive decoder follows them forever. readName must bail instead of spinning on this
	// untrusted multicast input.
	msg := make([]byte, 16)
	// pointer at offset 12 → 14
	msg[12] = 0xC0
	msg[13] = 14
	// pointer at offset 14 → 12
	msg[14] = 0xC0
	msg[15] = 12

	done := make(chan struct{})
	go func() {
		_, _, ok := readName(msg, 12)
		if ok {
			t.Errorf("readName(pointer loop) ok = true, want false")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("readName did not terminate on a pointer loop")
	}
}
