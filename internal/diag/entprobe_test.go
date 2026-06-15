package diag

import "testing"

func TestDTLSKind_classifiesClientHello(t *testing.T) {
	// Given: a DTLS 1.2 handshake record header (content-type 0x16, version 0xfefd)
	clientHello := []byte{0x16, 0xfe, 0xfd, 0x00, 0x00}

	// Then: it is recognized as a DTLS handshake the TV uses to open streaming
	if got := dtlsKind(clientHello); got == "" || got[:5] != "DTLS " {
		t.Fatalf("dtlsKind(ClientHello) = %q", got)
	}
}

func TestDTLSKind_classifiesNonHandshakeAndShort(t *testing.T) {
	if got := dtlsKind([]byte{0x17, 0x00, 0x00}); got == "" || got[:3] != "non" {
		t.Fatalf("dtlsKind(non-handshake) = %q", got)
	}
	if got := dtlsKind([]byte{0x16}); got != "too short" {
		t.Fatalf("dtlsKind(short) = %q", got)
	}
}
