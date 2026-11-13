package net

import (
	"encoding/gob"
	"fmt"
	 stdnet "net"
	"sync"
	"time"

	"soundcheck/internal/core"
)

// Server runs on the Host machine, accepting a co-op client connection over TCP (with SetNoDelay(true)),
// receiving Player 2 inputs at 30 Hz, and broadcasting WorldSnapshots at 30 Hz.
type Server struct {
	addr  string
	world *core.World

	mu      sync.Mutex
	client  stdnet.Conn
	encoder *gob.Encoder
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

			s.mu.Lock()
			if s.client != nil {
				_ = s.client.Close()
			}
			s.client = conn
			s.encoder = gob.NewEncoder(conn)
			s.mu.Unlock()

			s.world.SetClientConnected(true)
			go s.readClientLoop(conn)
		}
	}()

	return nil
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

// Client connects to a Host Server, sends PlayerInput 30 times/sec, and receives WorldSnapshots.
type Client struct {
	serverAddr string

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

func (c *Client) Start() {
	go func() {
		for {
			conn, err := stdnet.DialTimeout("tcp", c.serverAddr, 2*time.Second)
			if err != nil {
				c.mu.Lock()
				c.connected = false
				c.errStr = err.Error()
				c.mu.Unlock()
				time.Sleep(1 * time.Second)
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
