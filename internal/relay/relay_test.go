package relay

import (
	"encoding/json"
	"io"
	stdnet "net"
	"strings"
	"testing"
	"time"
)

func dialInMemory(rl *Relay) stdnet.Conn {
	c1, c2 := stdnet.Pipe()
	go rl.handleConn(c2)
	return c1
}

func TestRelayPublicPrivateListAndPipe(t *testing.T) {
	rl := New("127.0.0.1:0")
	defer rl.Stop()

	// 1. Create a private room
	privHost := dialInMemory(rl)
	defer privHost.Close()
	_, _ = io.WriteString(privHost, "HOST PRIVATE Secret Jam\n")
	privResp, err := readLine(privHost, 128)
	if err != nil || !strings.HasPrefix(privResp, "OK ") {
		t.Fatalf("expected OK <code> for private room, got %q (err=%v)", privResp, err)
	}
	privCode := strings.TrimPrefix(privResp, "OK ")

	// 2. Create a public room
	pubHost := dialInMemory(rl)
	defer pubHost.Close()
	_, _ = io.WriteString(pubHost, "HOST PUBLIC Open Stage\n")
	pubResp, err := readLine(pubHost, 128)
	if err != nil || !strings.HasPrefix(pubResp, "OK ") {
		t.Fatalf("expected OK <code> for public room, got %q (err=%v)", pubResp, err)
	}
	pubCode := strings.TrimPrefix(pubResp, "OK ")

	// 3. LIST should return only the public room
	listConn := dialInMemory(rl)
	_, _ = io.WriteString(listConn, "LIST\n")
	listJSON, err := readLine(listConn, 4096)
	_ = listConn.Close()
	if err != nil {
		t.Fatalf("read list: %v", err)
	}
	var rooms []RoomInfo
	if err := json.Unmarshal([]byte(listJSON), &rooms); err != nil {
		t.Fatalf("unmarshal list: %v (raw=%q)", err, listJSON)
	}
	if len(rooms) != 1 || rooms[0].Code != pubCode || rooms[0].Name != "Open Stage" {
		t.Fatalf("unexpected rooms list: %+v (privCode=%s, pubCode=%s)", rooms, privCode, pubCode)
	}

	// 4. Join invalid code should fail
	badConn := dialInMemory(rl)
	_, _ = io.WriteString(badConn, "JOIN ZZZZZ\n")
	badResp, err := readLine(badConn, 128)
	_ = badConn.Close()
	if err != nil || !strings.HasPrefix(badResp, "ERR ") {
		t.Fatalf("expected ERR for missing room, got %q (err=%v)", badResp, err)
	}

	// 5. Client joins public room (case-insensitive)
	hostJoinedCh := make(chan string, 1)
	go func() {
		_ = pubHost.SetReadDeadline(time.Now().Add(2 * time.Second))
		line, _ := readLine(pubHost, 128)
		_ = pubHost.SetReadDeadline(time.Time{})
		hostJoinedCh <- line
	}()

	clientConn := dialInMemory(rl)
	defer clientConn.Close()
	_, _ = io.WriteString(clientConn, "JOIN "+strings.ToLower(pubCode)+"\n")
	cResp, err := readLine(clientConn, 128)
	if err != nil || cResp != "OK" {
		t.Fatalf("expected OK on join, got %q (err=%v)", cResp, err)
	}

	if hResp := <-hostJoinedCh; hResp != "JOINED" {
		t.Fatalf("expected JOINED on host, got %q", hResp)
	}

	// 6. Verify bidirectional forwarding
	msgFromClient := "hello-from-client"
	go func() {
		_, _ = io.WriteString(clientConn, msgFromClient)
	}()
	buf := make([]byte, len(msgFromClient))
	_ = pubHost.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(pubHost, buf); err != nil {
		t.Fatalf("read host: %v", err)
	}
	if string(buf) != msgFromClient {
		t.Fatalf("got %q, want %q", string(buf), msgFromClient)
	}

	msgFromHost := "hello-from-host"
	go func() {
		_, _ = io.WriteString(pubHost, msgFromHost)
	}()
	buf2 := make([]byte, len(msgFromHost))
	_ = clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(clientConn, buf2); err != nil {
		t.Fatalf("read client: %v", err)
	}
	if string(buf2) != msgFromHost {
		t.Fatalf("got %q, want %q", string(buf2), msgFromHost)
	}
}
