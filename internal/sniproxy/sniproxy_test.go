package sniproxy

import (
	"strings"
	"testing"
)

func makeClientHello(sniName string) []byte {
	sni := []byte(sniName)
	sniLen := len(sni)

	sniExt := make([]byte, 0, 10+sniLen)
	sniExt = append(sniExt, 0, 0)                                             // extension type SNI
	sniExt = append(sniExt, byte((sniLen+5)>>8), byte(sniLen+5))             // extension data length
	sniExt = append(sniExt, byte((sniLen+3)>>8), byte(sniLen+3))             // server name list length
	sniExt = append(sniExt, 0)                                                // name type host_name
	sniExt = append(sniExt, byte(sniLen>>8), byte(sniLen))                    // name length
	sniExt = append(sniExt, sni...)                                           // name

	extLen := len(sniExt)

	random := make([]byte, 32)
	sessionID := make([]byte, 0)
	cipherSuites := []byte{0x00, 0x02, 0x13, 0x01}
	compression := []byte{0x01, 0x00}

	handshakePayload := make([]byte, 0, 38+len(cipherSuites)+len(compression)+2+extLen)
	handshakePayload = append(handshakePayload, 0x03, 0x03)
	handshakePayload = append(handshakePayload, random...)
	handshakePayload = append(handshakePayload, byte(len(sessionID)))
	handshakePayload = append(handshakePayload, sessionID...)
	handshakePayload = append(handshakePayload, byte(len(cipherSuites)>>8), byte(len(cipherSuites)))
	handshakePayload = append(handshakePayload, cipherSuites...)
	handshakePayload = append(handshakePayload, compression...)
	handshakePayload = append(handshakePayload, byte(extLen>>8), byte(extLen))
	handshakePayload = append(handshakePayload, sniExt...)

	hdl := len(handshakePayload)
	record := make([]byte, 0, 9+hdl)
	record = append(record, 0x16, 0x03, 0x01)
	record = append(record, byte((hdl+4)>>8), byte(hdl+4))
	record = append(record, 0x01)
	record = append(record, byte(hdl>>16), byte(hdl>>8), byte(hdl))
	record = append(record, handshakePayload...)
	return record
}

func TestExtractKnownDomains(t *testing.T) {
	tests := []struct {
		name string
		host string
	}{
		{"youtube", "youtube.com"},
		{"google", "google.com"},
		{"github", "github.com"},
		{"subdomain", "sub.domain.example.com"},
		{"single_label", "localhost"},
		{"xn-- punycode", "xn--mgba3a4f16a.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := makeClientHello(tt.host)
			got, _ := extract(data)
			if got != tt.host {
				t.Fatalf("extract(%q) = %q, want %q", tt.host, got, tt.host)
			}
		})
	}
}

func TestExtractEmptyInput(t *testing.T) {
	if got, _ := extract(nil); got != "" {
		t.Fatalf("extract(nil) = %q, want empty", got)
	}
	if got, _ := extract([]byte{}); got != "" {
		t.Fatalf("extract({}) = %q, want empty", got)
	}
	if got, _ := extract([]byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01, 0x00, 0x00, 0x00}); got != "" {
		t.Fatalf("extract(truncated) = %q, want empty", got)
	}
}

func TestExtractNotTLS(t *testing.T) {
	data := make([]byte, 50)
	data[0] = 0x17
	got, _ := extract(data)
	if got != "" {
		t.Fatalf("extract(not handshake) = %q, want empty", got)
	}
}

func TestExtractNotClientHello(t *testing.T) {
	data := makeClientHello("example.com")
	data[5] = 0x02
	got, _ := extract(data)
	if got != "" {
		t.Fatalf("extract(not client hello) = %q, want empty", got)
	}
}

func TestExtractNoSNI(t *testing.T) {
	data := makeClientHello("")
	got, _ := extract(data)
	if got != "" {
		t.Fatalf("extract(no sni) = %q, want empty", got)
	}
}

func TestExtractUppercaseSNI(t *testing.T) {
	data := makeClientHello("EXAMPLE.COM")
	got, _ := extract(data)
	if got != "example.com" {
		t.Fatalf("extract(upper) = %q, want %q", got, "example.com")
	}
}

func TestExtractMixedCaseSNI(t *testing.T) {
	data := makeClientHello("ExAmPlE.CoM")
	got, _ := extract(data)
	if got != "example.com" {
		t.Fatalf("extract(mixed) = %q, want %q", got, "example.com")
	}
}

func TestExtractAlreadyLowercase(t *testing.T) {
	data := makeClientHello("example.com")
	got, _ := extract(data)
	if got != "example.com" {
		t.Fatalf("extract(lower) = %q, want %q", got, "example.com")
	}
}

func TestBlocklistMatch(t *testing.T) {
	p := New(0)
	p.Block([]string{"youtube.com", "google.com"})

	b := p.bl.Load()
	if _, ok := b.m["youtube.com"]; !ok {
		t.Fatal("blocklist missing youtube.com")
	}
	if _, ok := b.m["google.com"]; !ok {
		t.Fatal("blocklist missing google.com")
	}
	if _, ok := b.m["example.com"]; ok {
		t.Fatal("blocklist should not contain example.com")
	}
}

func TestBlocklistUpdate(t *testing.T) {
	p := New(0)
	p.Block([]string{"old.com"})

	b := p.bl.Load()
	if _, ok := b.m["old.com"]; !ok {
		t.Fatal("blocklist should contain old.com")
	}
	_, ok := b.m["new.com"]
	if ok {
		t.Fatal("blocklist should not contain new.com yet")
	}

	p.Block([]string{"new.com"})

	b = p.bl.Load()
	if _, ok := b.m["new.com"]; !ok {
		t.Fatal("blocklist should contain new.com after update")
	}
	if _, ok := b.m["old.com"]; ok {
		t.Fatal("blocklist should not contain old.com after replacement")
	}
}

func TestExtractLargeSNI(t *testing.T) {
	long := strings.Repeat("a", 253) + ".com"
	data := makeClientHello(long)
	got, _ := extract(data)
	if got != long {
		t.Fatalf("extract(long) = %q, want %q", got, long)
	}
}

func makeClientHelloMulti(sniName string, leadingExts ...[]byte) []byte {
	sni := []byte(sniName)
	sniLen := len(sni)

	sniBody := make([]byte, 0, 5+sniLen)
	sniBody = append(sniBody, byte((sniLen+3)>>8), byte(sniLen+3))
	sniBody = append(sniBody, 0)
	sniBody = append(sniBody, byte(sniLen>>8), byte(sniLen))
	sniBody = append(sniBody, sni...)

	sniExt := make([]byte, 0, 4+len(sniBody))
	sniExt = append(sniExt, 0, 0)
	sniExt = append(sniExt, byte(len(sniBody)>>8), byte(len(sniBody)))
	sniExt = append(sniExt, sniBody...)

	fullExt := make([]byte, 0, 4)
	for _, e := range leadingExts {
		fullExt = append(fullExt, e...)
	}
	fullExt = append(fullExt, sniExt...)

	random := make([]byte, 32)
	sessionID := make([]byte, 0)
	cipherSuites := []byte{0x00, 0x02, 0x13, 0x01}
	compression := []byte{0x01, 0x00}

	handshakePayload := make([]byte, 0, 38+len(cipherSuites)+len(compression)+2+len(fullExt))
	handshakePayload = append(handshakePayload, 0x03, 0x03)
	handshakePayload = append(handshakePayload, random...)
	handshakePayload = append(handshakePayload, byte(len(sessionID)))
	handshakePayload = append(handshakePayload, sessionID...)
	handshakePayload = append(handshakePayload, byte(len(cipherSuites)>>8), byte(len(cipherSuites)))
	handshakePayload = append(handshakePayload, cipherSuites...)
	handshakePayload = append(handshakePayload, compression...)
	handshakePayload = append(handshakePayload, byte(len(fullExt)>>8), byte(len(fullExt)))
	handshakePayload = append(handshakePayload, fullExt...)

	hdl := len(handshakePayload)
	record := make([]byte, 0, 9+hdl)
	record = append(record, 0x16, 0x03, 0x01)
	record = append(record, byte((hdl+4)>>8), byte(hdl+4))
	record = append(record, 0x01)
	record = append(record, byte(hdl>>16), byte(hdl>>8), byte(hdl))
	record = append(record, handshakePayload...)
	return record
}

func TestExtractMultipleExtensionsSNILast(t *testing.T) {
	dummy1 := []byte{0x00, 0x01, 0x00, 0x02, 0xFF, 0xFF}
	dummy2 := []byte{0x00, 0x10, 0x00, 0x04, 0xAA, 0xBB, 0xCC, 0xDD}
	data := makeClientHelloMulti("example.com", dummy1, dummy2)
	got, _ := extract(data)
	if got != "example.com" {
		t.Fatalf("extract with leading extensions = %q, want %q", got, "example.com")
	}
}

func TestExtractSNIMidExtensions(t *testing.T) {
	emptyExt := []byte{0x00, 0x01, 0x00, 0x00}
	data := makeClientHelloMulti("example.com", emptyExt)
	got, _ := extract(data)
	if got != "example.com" {
		t.Fatalf("extract with SNI mid extensions = %q, want %q", got, "example.com")
	}
}

func TestExtractECHDetected(t *testing.T) {
	echExt := []byte{0xFE, 0x0D, 0x00, 0x05, 0x01, 0x02, 0x03, 0x04, 0x05}
	data := makeClientHelloMulti("youtube.com", echExt)
	_, ech := extract(data)
	if !ech {
		t.Fatal("extract should detect ECH extension")
	}
}

func TestExtractECHWithouSNI(t *testing.T) {
	echExt := []byte{0xFE, 0x0D, 0x00, 0x02, 0x00, 0x00}
	data := makeClientHelloMulti("", echExt)
	sni, ech := extract(data)
	if !ech {
		t.Fatal("extract should detect ECH extension")
	}
	if sni != "" {
		t.Fatalf("extract should return empty SNI with ECH, got %q", sni)
	}
}

func TestExtractECHMultipleExtensions(t *testing.T) {
	dummy1 := []byte{0x00, 0x01, 0x00, 0x02, 0xFF, 0xFF}
	echExt := []byte{0xFE, 0x0D, 0x00, 0x04, 0xAA, 0xBB, 0xCC, 0xDD}
	data := makeClientHelloMulti("example.com", dummy1, echExt)
	got, ech := extract(data)
	if !ech {
		t.Fatal("extract should detect ECH among other extensions")
	}
	if got != "example.com" {
		t.Fatalf("extract should still return outer SNI with ECH, got %q", got)
	}
}

func TestExtractNoECH(t *testing.T) {
	data := makeClientHello("example.com")
	_, ech := extract(data)
	if ech {
		t.Fatal("extract should not detect ECH when not present")
	}
}

func BenchmarkExtract(b *testing.B) {
	data := makeClientHello("youtube.com")
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s, _ := extract(data)
		if s != "youtube.com" {
			b.Fatalf("unexpected result: %s", s)
		}
	}
}

func BenchmarkBlocklistLookup(b *testing.B) {
	p := New(0)
	p.Block([]string{"youtube.com", "google.com", "reddit.com", "facebook.com", "instagram.com"})
	b.ResetTimer()
	b.ReportAllocs()
	total := 0
	for i := 0; i < b.N; i++ {
		bl := p.bl.Load()
		if _, ok := bl.m["youtube.com"]; ok {
			total++
		}
	}
	_ = total
}

func BenchmarkBlocklistFull(b *testing.B) {
	data := makeClientHello("youtube.com")
	p := New(0)
	p.Block([]string{"youtube.com", "google.com", "reddit.com", "facebook.com", "instagram.com", "x.com", "netflix.com", "tiktok.com", "twitch.tv"})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s, _ := extract(data)
		bl := p.bl.Load()
		if _, ok := bl.m[s]; ok {
		}
	}
}

func TestBe16(t *testing.T) {
	tests := []struct {
		input []byte
		want  int
	}{
		{[]byte{0x00, 0x00}, 0},
		{[]byte{0x00, 0x01}, 1},
		{[]byte{0x01, 0x00}, 256},
		{[]byte{0xff, 0xff}, 65535},
		{[]byte{0x12, 0x34}, 4660},
	}

	for _, tt := range tests {
		got := be16(tt.input)
		if got != tt.want {
			t.Fatalf("be16(%#v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{80, "80"},
		{443, "443"},
		{8443, "8443"},
		{65535, "65535"},
	}

	for _, tt := range tests {
		got := itoa(tt.input)
		if got != tt.want {
			t.Fatalf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
