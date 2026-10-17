package main

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// LAN discovery: the host shouts "CEITG|name|players" via UDP broadcast once a second,
// joiners listen and show the games they hear about.

type FoundGame struct {
	Addr    string // ip
	Name    string
	Players int
	seen    time.Time
}

func Announce(stop <-chan struct{}, hostName string, count func() int) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return
	}
	defer conn.Close()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		msg := []byte(fmt.Sprintf("CEITG|%s|%d", hostName, count()))
		for _, b := range broadcastAddrs() {
			conn.WriteToUDP(msg, &net.UDPAddr{IP: b, Port: DiscPort})
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

func broadcastAddrs() []net.IP {
	out := []net.IP{net.IPv4bcast}
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			n, ok := a.(*net.IPNet)
			if !ok || n.IP.To4() == nil {
				continue
			}
			ip, m := n.IP.To4(), n.Mask
			if len(m) == 16 {
				m = m[12:]
			}
			b := make(net.IP, 4)
			for i := range b {
				b[i] = ip[i] | ^m[i]
			}
			out = append(out, b)
		}
	}
	return out
}

// LocalIPs returns this machine's LAN IPv4 addresses (shown to the host).
func LocalIPs() []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n.IP.String())
		}
	}
	return out
}

type Finder struct {
	mu    sync.Mutex
	games map[string]*FoundGame
	conn  *net.UDPConn
}

func NewFinder() *Finder {
	f := &Finder{games: map[string]*FoundGame{}}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: DiscPort})
	if err != nil {
		return f
	}
	f.conn = conn
	go func() {
		buf := make([]byte, 256)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			parts := strings.Split(string(buf[:n]), "|")
			if len(parts) != 3 || parts[0] != "CEITG" {
				continue
			}
			cnt, _ := strconv.Atoi(parts[2])
			ip := from.IP.String()
			f.mu.Lock()
			f.games[ip] = &FoundGame{Addr: ip, Name: parts[1], Players: cnt, seen: time.Now()}
			f.mu.Unlock()
		}
	}()
	return f
}

func (f *Finder) List() []FoundGame {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []FoundGame
	for k, g := range f.games {
		if time.Since(g.seen) > 3500*time.Millisecond {
			delete(f.games, k)
			continue
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Addr < out[j].Addr })
	return out
}

func (f *Finder) Close() {
	if f.conn != nil {
		f.conn.Close()
	}
}
