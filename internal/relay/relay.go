package relay

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	stdnet "net"
	"sort"
	"strings"
	"sync"
	"time"
)

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// RoomInfo describes a public room waiting for a partner.
type RoomInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type pairRequest struct {
	conn stdnet.Conn
	resp chan error
}

type room struct {
	code      string
	name      string
	public    bool
	paired    bool
	createdAt time.Time
	pairCh    chan pairRequest
	closedCh  chan struct{}
}

// Relay accepts TCP connections from game hosts and clients,
// manages room codes and public room listings, and pipes traffic between paired peers.
type Relay struct {
	listenAddr string

	mu       sync.Mutex
	ln       stdnet.Listener
	rooms    map[string]*room
	stopOnce sync.Once
	stopCh   chan struct{}
}

func New(listenAddr string) *Relay {
	return &Relay{
		listenAddr: listenAddr,
		rooms:      make(map[string]*room),
		stopCh:     make(chan struct{}),
	}
}

func (r *Relay) Start() error {
	ln, err := stdnet.Listen("tcp", r.listenAddr)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.ln = ln
	r.mu.Unlock()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				select {
				case <-r.stopCh:
					return
				default:
					continue
				}
			}
			if tcpConn, ok := conn.(*stdnet.TCPConn); ok {
				_ = tcpConn.SetNoDelay(true)
			}
			go r.handleConn(conn)
		}
	}()

	return nil
}

func (r *Relay) Addr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ln != nil {
		return r.ln.Addr().String()
	}
	return r.listenAddr
}

func (r *Relay) Stop() {
	r.stopOnce.Do(func() {
		close(r.stopCh)
		r.mu.Lock()
		if r.ln != nil {
			_ = r.ln.Close()
		}
		r.mu.Unlock()
	})
}

func (r *Relay) handleConn(conn stdnet.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := readLine(conn, 512)
	if err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	fields := strings.SplitN(strings.TrimSpace(line), " ", 3)
	if len(fields) == 0 {
		_ = conn.Close()
		return
	}

	cmd := strings.ToUpper(fields[0])
	switch cmd {
	case "LIST":
		r.handleList(conn)
	case "HOST":
		public := len(fields) >= 2 && strings.ToUpper(fields[1]) == "PUBLIC"
		name := "Soundcheck Room"
		if len(fields) >= 3 && strings.TrimSpace(fields[2]) != "" {
			name = strings.TrimSpace(fields[2])
		}
		r.handleHost(conn, public, name)
	case "JOIN":
		if len(fields) < 2 {
			_, _ = io.WriteString(conn, "ERR missing room code\n")
			_ = conn.Close()
			return
		}
		code := strings.ToUpper(strings.TrimSpace(fields[1]))
		r.handleJoin(conn, code)
	default:
		_, _ = io.WriteString(conn, "ERR unknown command\n")
		_ = conn.Close()
	}
}

func (r *Relay) handleList(conn stdnet.Conn) {
	defer conn.Close()

	r.mu.Lock()
	var list []RoomInfo
	for _, rm := range r.rooms {
		if rm.public && !rm.paired {
			list = append(list, RoomInfo{
				Code: rm.code,
				Name: rm.name,
			})
		}
	}
	r.mu.Unlock()

	sort.Slice(list, func(i, j int) bool {
		return list[i].Code < list[j].Code
	})

	data, err := json.Marshal(list)
	if err != nil {
		_, _ = io.WriteString(conn, "[]\n")
		return
	}
	_, _ = conn.Write(append(data, '\n'))
}

func (r *Relay) handleHost(hostConn stdnet.Conn, public bool, name string) {
	rm, err := r.createRoom(public, name)
	if err != nil {
		_, _ = io.WriteString(hostConn, "ERR "+err.Error()+"\n")
		_ = hostConn.Close()
		return
	}

	defer func() {
		r.removeRoom(rm.code, rm)
		_ = hostConn.Close()
	}()

	if _, err := io.WriteString(hostConn, "OK "+rm.code+"\n"); err != nil {
		return
	}
	log.Printf("relay: room %s created (public=%v, name=%q)", rm.code, public, name)

	// Wait for either a client to pair or host to disconnect.
	var probe [1]byte
	for {
		select {
		case <-r.stopCh:
			return
		case req := <-rm.pairCh:
			_ = hostConn.SetReadDeadline(time.Time{})
			if _, err := io.WriteString(hostConn, "JOINED\n"); err != nil {
				req.resp <- err
				return
			}
			if _, err := io.WriteString(req.conn, "OK\n"); err != nil {
				req.resp <- err
				return
			}
			req.resp <- nil
			log.Printf("relay: room %s paired", rm.code)
			pipe(hostConn, req.conn)
			log.Printf("relay: room %s closed", rm.code)
			return
		default:
			_ = hostConn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			_, err := hostConn.Read(probe[:])
			if err != nil {
				var ne stdnet.Error
				if errors.As(err, &ne) && ne.Timeout() {
					continue
				}
				log.Printf("relay: room %s host disconnected while waiting", rm.code)
				return
			}
		}
	}
}

func (r *Relay) handleJoin(clientConn stdnet.Conn, code string) {
	r.mu.Lock()
	rm, ok := r.rooms[code]
	if !ok {
		r.mu.Unlock()
		_, _ = io.WriteString(clientConn, "ERR room not found\n")
		_ = clientConn.Close()
		return
	}
	if rm.paired {
		r.mu.Unlock()
		_, _ = io.WriteString(clientConn, "ERR room already in game\n")
		_ = clientConn.Close()
		return
	}
	rm.paired = true
	r.mu.Unlock()

	req := pairRequest{
		conn: clientConn,
		resp: make(chan error, 1),
	}

	select {
	case rm.pairCh <- req:
	case <-rm.closedCh:
		_, _ = io.WriteString(clientConn, "ERR room closed\n")
		_ = clientConn.Close()
		return
	case <-time.After(2 * time.Second):
		_, _ = io.WriteString(clientConn, "ERR room busy\n")
		_ = clientConn.Close()
		return
	}

	select {
	case err := <-req.resp:
		if err != nil {
			_, _ = io.WriteString(clientConn, "ERR "+err.Error()+"\n")
			_ = clientConn.Close()
		}
	case <-time.After(3 * time.Second):
		_, _ = io.WriteString(clientConn, "ERR handshake timeout\n")
		_ = clientConn.Close()
	}
}

func (r *Relay) createRoom(public bool, name string) (*room, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for attempt := 0; attempt < 32; attempt++ {
		code, err := generateCode(5)
		if err != nil {
			return nil, err
		}
		if _, exists := r.rooms[code]; !exists {
			rm := &room{
				code:      code,
				name:      name,
				public:    public,
				createdAt: time.Now(),
				pairCh:    make(chan pairRequest),
				closedCh:  make(chan struct{}),
			}
			r.rooms[code] = rm
			return rm, nil
		}
	}
	return nil, fmt.Errorf("failed to allocate room code")
}

func (r *Relay) removeRoom(code string, rm *room) {
	r.mu.Lock()
	if cur, ok := r.rooms[code]; ok && cur == rm {
		delete(r.rooms, code)
		close(rm.closedCh)
	}
	r.mu.Unlock()
}

func generateCode(n int) (string, error) {
	b := make([]byte, n)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := 0; i < n; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = codeAlphabet[v.Int64()]
	}
	return string(b), nil
}

func pipe(a, b stdnet.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	copyHalf := func(dst, src stdnet.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		_ = dst.Close()
		_ = src.Close()
	}
	go copyHalf(a, b)
	go copyHalf(b, a)
	wg.Wait()
}

// readLine reads a single '\n'-terminated line one byte at a time so no buffered bytes
// after '\n' are consumed from the underlying connection.
func readLine(r io.Reader, maxLen int) (string, error) {
	buf := make([]byte, 0, 64)
	var b [1]byte
	for len(buf) < maxLen {
		n, err := r.Read(b[:])
		if n > 0 {
			if b[0] == '\n' {
				return strings.TrimRight(string(buf), "\r"), nil
			}
			buf = append(buf, b[0])
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("line too long")
}
