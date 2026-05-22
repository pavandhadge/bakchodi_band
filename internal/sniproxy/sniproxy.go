package sniproxy

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type Proxy struct {
	bl     atomic.Pointer[blockmap]
	l      net.Listener
	pool   sync.Pool
	cpool  sync.Pool
	chpool sync.Pool
}

type blockmap struct {
	m map[string]struct{}
}

func New(port int) *Proxy {
	p := &Proxy{}
	p.bl.Store(&blockmap{make(map[string]struct{})})
	p.pool.New = func() any { b := make([]byte, 9000); return &b }
	p.cpool.New = func() any { b := make([]byte, 32768); return &b }
	p.chpool.New = func() any { b := make([]byte, 4096); return &b }
	l, _ := net.Listen("tcp", net.JoinHostPort("", itoa(port)))
	p.l = l
	return p
}

func (p *Proxy) Block(domains []string) {
	m := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		m[strings.ToLower(d)] = struct{}{}
	}
	p.bl.Store(&blockmap{m})
}

func (p *Proxy) Open() {
	if p.l == nil {
		return
	}
	go func() {
		for {
			c, e := p.l.Accept()
			if e != nil {
				return
			}
			go p.hand(c)
		}
	}()
}

func (p *Proxy) Close() {
	if p.l != nil {
		p.l.Close()
	}
}

func (p *Proxy) hand(c net.Conn) {
	defer c.Close()

	c.SetReadDeadline(time.Now().Add(10 * time.Second))
	defer c.SetReadDeadline(time.Time{})

	bp := p.pool.Get().(*[]byte)
	buf := *bp

	n, _ := c.Read(buf[:9])
	if n < 5 {
		return
	}

	if buf[0] == 0x16 {
		p.handTLS(c, buf, n)
		return
	}

	if n >= 7 && bytes.EqualFold(buf[:7], []byte("CONNECT")) {
		p.handConnect(c, buf[:n])
	}
}

func (p *Proxy) handTLS(c net.Conn, buf []byte, n int) {
	rl := int(buf[3])<<8 | int(buf[4])
	need := 5 + rl
	if need > len(buf) {
		need = len(buf)
	}
	for n < need {
		m, _ := c.Read(buf[n:need])
		if m == 0 {
			return
		}
		n += m
	}

	sni, ech := extract(buf[:n])
	if ech {
		return
	}
	if sni != "" {
		b := p.bl.Load()
		if _, ok := b.m[sni]; ok {
			return
		}
	}

	dst, err := getOriginalDest(c)
	if err != nil {
		return
	}

	up, err := net.DialTimeout("tcp", dst, 3*time.Second)
	if err != nil {
		return
	}
	defer up.Close()

	up.Write(buf[:n])
	p.pipe(up, c)
}

func (p *Proxy) handConnect(c net.Conn, initial []byte) {
	bpp := p.chpool.Get().(*[]byte)
	buf := *bpp
	pos := copy(buf, initial)
	for {
		n, err := c.Read(buf[pos:])
		if n == 0 || err != nil {
			return
		}
		pos += n
		if bytes.Contains(buf[:pos], []byte("\r\n\r\n")) {
			break
		}
		if pos >= len(buf) {
			return
		}
	}

	end := bytes.Index(buf[:pos], []byte("\r\n"))
	if end < 0 {
		return
	}
	line := buf[:end]
	sp1 := bytes.IndexByte(line, ' ')
	if sp1 < 0 {
		return
	}
	cmd := line[:sp1]
	if !bytes.EqualFold(cmd, []byte("CONNECT")) {
		return
	}
	rest := line[sp1+1:]
	sp2 := bytes.IndexByte(rest, ' ')
	if sp2 < 0 {
		return
	}
	target := rest[:sp2]

	colon := bytes.IndexByte(target, ':')
	var host string
	if colon >= 0 {
		host = string(target[:colon])
	} else {
		host = string(target)
	}

	b := p.bl.Load()
	if _, ok := b.m[toLower(host)]; ok {
		c.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\n"))
		return
	}

	up, err := net.DialTimeout("tcp", string(target), 3*time.Second)
	if err != nil {
		return
	}
	defer up.Close()

	c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	p.pipe(up, c)
}

func (p *Proxy) pipe(up, c net.Conn) {
	cp := p.cpool.Get().(*[]byte)
	cpy := *cp

	go func() {
		io.CopyBuffer(up, c, cpy)
		up.Close()
	}()

	io.CopyBuffer(c, up, cpy)
}

func extract(d []byte) (string, bool) {
	if len(d) < 6 || d[0] != 0x16 || d[5] != 1 {
		return "", false
	}
	hl := int(d[6])<<16 | int(d[7])<<8 | int(d[8])
	if 9+hl > len(d) {
		return "", false
	}
	b := d[9 : 9+hl]
	if len(b) < 39 {
		return "", false
	}

	off := 34
	off += 1 + int(b[off])
	off += 2 + (int(b[off])<<8 | int(b[off+1]))
	if off > len(b) {
		return "", false
	}
	off += 1 + int(b[off])
	if off+2 > len(b) {
		return "", false
	}
	el := int(b[off])<<8 | int(b[off+1])
	off += 2
	end := off + el
	if end > len(b) {
		end = len(b)
	}
	if off+4 > end {
		return "", false
	}

	ech := false
	sniName := ""

	t := int(b[off])<<8 | int(b[off+1])
	l := int(b[off+2])<<8 | int(b[off+3])

	if t == 0 && l > 5 && off+4+l <= end {
		ll := int(b[off+4])<<8 | int(b[off+5])
		if ll > 2 && off+6+ll <= off+4+l && b[off+6] == 0 {
			nl := int(b[off+7])<<8 | int(b[off+8])
			pos := off + 9
			if nl > 0 && pos+nl <= off+4+l {
				for j := 0; j < nl; j++ {
					if b[pos+j] >= 'A' && b[pos+j] <= 'Z' {
						s := unsafe.String(&b[pos], nl)
						sniName = strings.ToLower(s)
						goto next
					}
				}
				sniName = unsafe.String(&b[pos], nl)
			}
		}
	next:
		off += 4 + l
	} else {
		if t == 0xFE0D {
			ech = true
		}
		off += 4 + l
	}

	for off+4 <= end {
		t := int(b[off])<<8 | int(b[off+1])
		l := int(b[off+2])<<8 | int(b[off+3])
		off += 4
		if t == 0xFE0D {
			ech = true
		}
		if t == 0 && l > 5 && off+l <= end && sniName == "" {
			sniName = nameFromSNI(b, off, off+l)
		}
		off += l
	}
	return sniName, ech
}



func nameFromSNI(b []byte, off, end int) string {
	ll := int(b[off])<<8 | int(b[off+1])
	send := off + 2 + ll
	if send > end {
		send = end
	}
	for i := off + 2; i+3 < send; {
		if b[i] != 0 {
			i += 3 + (int(b[i+1])<<8 | int(b[i+2]))
			continue
		}
		nl := int(b[i+1])<<8 | int(b[i+2])
		pos := i + 3
		if nl > 0 && pos+nl <= send {
			for j := 0; j < nl; j++ {
				if b[pos+j] >= 'A' && b[pos+j] <= 'Z' {
					s := unsafe.String(&b[pos], nl)
					return strings.ToLower(s)
				}
			}
			return unsafe.String(&b[pos], nl)
		}
		break
	}
	return ""
}

func toLower(s string) string {
	for i := range s {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return strings.ToLower(s)
		}
	}
	return s
}

func be16(b []byte) int {
	return int(b[0])<<8 | int(b[1])
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [5]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte(i%10) + '0'
		i /= 10
	}
	return string(b[p:])
}
