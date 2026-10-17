package main

import (
	"bufio"
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"
)

type Client struct {
	conn    net.Conn
	mu      sync.Mutex
	myID    int
	state   ServerMsg
	fresh   bool
	err     error
	lastIn  uint8
	sentAgo int
	outCh   chan uint8
}

func Connect(addr, name string) (*Client, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, strconv.Itoa(GamePort)), 3*time.Second)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, outCh: make(chan uint8, 8)}
	if err := c.send(ClientMsg{T: "hello", Name: name}); err != nil {
		conn.Close()
		return nil, err
	}
	go c.readLoop()
	go func() {
		for k := range c.outCh {
			if c.send(ClientMsg{T: "in", K: k}) != nil {
				return
			}
		}
	}()
	return c, nil
}

func (c *Client) readLoop() {
	r := bufio.NewReaderSize(c.conn, 64*1024)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			c.mu.Lock()
			c.err = err
			c.mu.Unlock()
			return
		}
		var m ServerMsg
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		c.mu.Lock()
		if m.T == "welcome" {
			c.myID = m.ID
		} else {
			c.state, c.fresh = m, true
		}
		c.mu.Unlock()
	}
}

func (c *Client) send(m ClientMsg) error {
	b, _ := json.Marshal(m)
	c.conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err := c.conn.Write(append(b, '\n'))
	return err
}

// SendInput only talks to the server when keys change (plus a slow keep-alive).
func (c *Client) SendInput(k uint8) {
	c.sentAgo++
	if k == c.lastIn && c.sentAgo < 30 {
		return
	}
	c.lastIn, c.sentAgo = k, 0
	select {
	case c.outCh <- k:
	default:
	}
}

// Snapshot returns the latest server state, whether it is new, and any connection error.
func (c *Client) Snapshot() (ServerMsg, int, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	f := c.fresh
	c.fresh = false
	return c.state, c.myID, f, c.err
}

func (c *Client) Close() {
	c.conn.Close()
	close(c.outCh)
}
