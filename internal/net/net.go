package net

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	stdnet "net"
	"strings"
	"sync"
	"time"

	"soundcheck/internal/core"
	"soundcheck/internal/relay"
)

// DefaultRelayAddr is the default relay server address used when none is specified.
const DefaultRelayAddr = "212.116.234.17:7778"

// Server runs on the Host machine, accepting a co-op client connection over TCP (with SetNoDelay(true)),
// either directly or via a Relay server, receiving Player 2 inputs at 30 Hz and broadcasting WorldSnapshots at 30 Hz.
type Server struct {
	addr  string
	world *core.World

	mu        sync.Mutex
	client    stdnet.Conn
	encoder   *gob.Encoder
	relayCode string
	relayErr  string
}

func NewServer(port int, world *core.World) *Server {
	return &Server{
		addr:  fmt.Sprintf(":%d", port),
		world: world,
	}
}

func (s *Server) Start() error {
	ln, err := stdnet.Listen("tcp", s.addr)
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				continue
			}
			if tcpConn, ok := conn.(*stdnet.TCPConn); ok {
				_ = tcpConn.SetNoDelay(true)
			}

			s.attachClient(conn)
			go s.readClientLoop(conn)
		}
	}()

	return nil
}

func (s *Server) attachClient(conn stdnet.Conn) {
	s.mu.Lock()
	if s.client != nil {
		_ = s.client.Close()
	}
	s.client = conn
	s.encoder = gob.NewEncoder(conn)
	s.mu.Unlock()

	s.world.SetClientConnected(true)
}

// StartRelay registers a room on the relay server and waits for a client to join.
// Returns the assigned room code on success.
func (s *Server) StartRelay(relayAddr string, public bool, roomName string) (string, error) {
	conn, err := stdnet.DialTimeout("tcp", relayAddr, 4*time.Second)
	if err != nil {
		return "", err
	}
	if tcpConn, ok := conn.(*stdnet.TCPConn); ok {
		_ = tcpConn.SetNoDelay(true)
	}

	vis := "PRIVATE"
	if public {
		vis = "PUBLIC"
	}
	cmd := fmt.Sprintf("HOST %s %s\n", vis, strings.TrimSpace(roomName))
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
	if _, err := io.WriteString(conn, cmd); err != nil {
		_ = conn.Close()
		return "", err
	}
	_ = conn.SetWriteDeadline(time.Time{})

	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	resp, err := readLine(conn, 256)
	if err != nil {
		_ = conn.Close()
		return "", err
	}
	_ = conn.SetReadDeadline(time.Time{})

	if !strings.HasPrefix(resp, "OK ") {
		_ = conn.Close()
		return "", fmt.Errorf("%s", strings.TrimPrefix(resp, "ERR "))
	}
	code := strings.TrimSpace(strings.TrimPrefix(resp, "OK "))

	s.mu.Lock()
	s.relayCode = code
	s.relayErr = ""
	s.mu.Unlock()

	go s.waitRelayLoop(conn, relayAddr, public, roomName)
	return code, nil
}

func (s *Server) waitRelayLoop(conn stdnet.Conn, relayAddr string, public bool, roomName string) {
	for {
		line, err := readLine(conn, 128)
		if err != nil || line != "JOINED" {
			_ = conn.Close()
			s.mu.Lock()
			s.relayErr = "relay disconnected"
			s.mu.Unlock()
			return
		}

		s.attachClient(conn)
		s.readClientLoop(conn)

		// Partner disconnected; reopen a new room on the relay automatically so Host stays reachable.
		for {
			time.Sleep(800 * time.Millisecond)
			newConn, dialErr := stdnet.DialTimeout("tcp", relayAddr, 4*time.Second)
			if dialErr != nil {
				s.mu.Lock()
				s.relayErr = dialErr.Error()
				s.mu.Unlock()
				continue
			}
			if tcpConn, ok := newConn.(*stdnet.TCPConn); ok {
				_ = tcpConn.SetNoDelay(true)
			}
			vis := "PRIVATE"
			if public {
				vis = "PUBLIC"
			}
			_, writeErr := io.WriteString(newConn, fmt.Sprintf("HOST %s %s\n", vis, strings.TrimSpace(roomName)))
			if writeErr != nil {
				_ = newConn.Close()
				continue
			}
			_ = newConn.SetReadDeadline(time.Now().Add(4 * time.Second))
			resp, readErr := readLine(newConn, 256)
			_ = newConn.SetReadDeadline(time.Time{})
			if readErr != nil || !strings.HasPrefix(resp, "OK ") {
				_ = newConn.Close()
				continue
			}
			s.mu.Lock()
			s.relayCode = strings.TrimSpace(strings.TrimPrefix(resp, "OK "))
			s.relayErr = ""
			s.mu.Unlock()
			conn = newConn
			break
		}
	}
}

func (s *Server) RelayCode() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.relayCode
}

func (s *Server) RelayErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.relayErr
}

func (s *Server) readClientLoop(conn stdnet.Conn) {
	dec := gob.NewDecoder(conn)
	for {
		var inp core.PlayerInput
		if err := dec.Decode(&inp); err != nil {
			break
		}
		s.world.SetInput(1, inp)
	}

	s.mu.Lock()
	if s.client == conn {
		s.client = nil
		s.encoder = nil
		s.world.SetClientConnected(false)
	}
	s.mu.Unlock()
	_ = conn.Close()
}

func (s *Server) BroadcastSnapshot(snap core.WorldSnapshot) {
	s.mu.Lock()
	enc := s.encoder
	conn := s.client
	s.mu.Unlock()

	if enc == nil || conn == nil {
		return
	}

	_ = conn.SetWriteDeadline(time.Now().Add(80 * time.Millisecond))
	if err := enc.Encode(snap); err != nil {
		s.mu.Lock()
		if s.client == conn {
			s.client = nil
			s.encoder = nil
			s.world.SetClientConnected(false)
		}
		s.mu.Unlock()
		_ = conn.Close()
	}
}

// Client connects to a Host Server (directly or via Relay), sends PlayerInput 30 times/sec, and receives WorldSnapshots.
type Client struct {
	serverAddr string
	relayAddr  string
	roomCode   string

	mu            sync.Mutex
	conn          stdnet.Conn
	enc           *gob.Encoder
	latestSnap    core.WorldSnapshot
	hasSnap       bool
	pendingSounds []core.SoundEvent
	connected     bool
	errStr        string
}

func NewClient(serverAddr string) *Client {
	return &Client{
		serverAddr: serverAddr,
	}
}

func NewRelayClient(relayAddr, roomCode string) *Client {
	return &Client{
		relayAddr: relayAddr,
		roomCode:  strings.ToUpper(strings.TrimSpace(roomCode)),
	}
}

func (c *Client) dialTarget() (stdnet.Conn, error) {
	if c.roomCode == "" {
		return stdnet.DialTimeout("tcp", c.serverAddr, 2*time.Second)
	}
	conn, err := stdnet.DialTimeout("tcp", c.relayAddr, 4*time.Second)
	if err != nil {
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Now().Add(4 * time.Second))
	if _, err := io.WriteString(conn, "JOIN "+c.roomCode+"\n"); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetWriteDeadline(time.Time{})

	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	resp, err := readLine(conn, 256)
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp != "OK" {
		_ = conn.Close()
		return nil, fmt.Errorf("%s", strings.TrimPrefix(resp, "ERR "))
	}
	return conn, nil
}

func (c *Client) Start() {
	go func() {
		for {
			conn, err := c.dialTarget()
			if err != nil {
				c.mu.Lock()
				c.connected = false
				c.errStr = err.Error()
				c.mu.Unlock()
				time.Sleep(1200 * time.Millisecond)
				continue
			}
			if tcpConn, ok := conn.(*stdnet.TCPConn); ok {
				_ = tcpConn.SetNoDelay(true)
			}

			c.mu.Lock()
			c.conn = conn
			c.enc = gob.NewEncoder(conn)
			c.connected = true
			c.errStr = ""
			c.mu.Unlock()

			dec := gob.NewDecoder(conn)
			for {
				var snap core.WorldSnapshot
				if err := dec.Decode(&snap); err != nil {
					break
				}
				c.mu.Lock()
				if len(snap.Sounds) > 0 {
					c.pendingSounds = append(c.pendingSounds, snap.Sounds...)
				}
				snap.Sounds = nil
				c.latestSnap = snap
				c.hasSnap = true
				c.mu.Unlock()
			}

			c.mu.Lock()
			c.connected = false
			c.conn = nil
			c.enc = nil
			c.mu.Unlock()
			_ = conn.Close()
			time.Sleep(1 * time.Second)
		}
	}()
}

func (c *Client) SendInput(inp core.PlayerInput) {
	c.mu.Lock()
	enc := c.enc
	conn := c.conn
	c.mu.Unlock()

	if enc == nil || conn == nil {
		return
	}
	_ = conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
	_ = enc.Encode(inp)
}

func (c *Client) GetSnapshot() (core.WorldSnapshot, bool, bool, string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := c.latestSnap
	if len(c.pendingSounds) > 0 {
		snap.Sounds = make([]core.SoundEvent, len(c.pendingSounds))
		copy(snap.Sounds, c.pendingSounds)
		c.pendingSounds = c.pendingSounds[:0]
	}
	return snap, c.hasSnap, c.connected, c.errStr
}

// FetchPublicRooms queries the relay server for open public rooms waiting for a partner.
func FetchPublicRooms(relayAddr string) ([]relay.RoomInfo, error) {
	conn, err := stdnet.DialTimeout("tcp", relayAddr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.WriteString(conn, "LIST\n"); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := readLine(conn, 64*1024)
	if err != nil {
		return nil, err
	}

	var rooms []relay.RoomInfo
	if err := json.Unmarshal([]byte(line), &rooms); err != nil {
		return nil, err
	}
	return rooms, nil
}

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
