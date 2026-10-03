//go:build phase29preflight

package app

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A byte-for-byte TCP relay: no model or storage response is synthesized.
type liveNetworkProxy struct {
	listener    net.Listener
	target      string
	delay       atomic.Int64
	broken      atomic.Bool
	mu          sync.Mutex
	connections map[net.Conn]bool
	workers     sync.WaitGroup
	t           *testing.T
}

func newLiveNetworkProxy(t *testing.T, target string) *liveNetworkProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxy := &liveNetworkProxy{listener: listener, target: target, connections: map[net.Conn]bool{}, t: t}
	proxy.workers.Add(1)
	go proxy.accept()
	t.Cleanup(proxy.close)
	return proxy
}

func (proxy *liveNetworkProxy) accept() {
	defer proxy.workers.Done()
	for {
		connection, err := proxy.listener.Accept()
		if err != nil {
			return
		}
		proxy.workers.Add(1)
		go proxy.relay(connection)
	}
}

func (proxy *liveNetworkProxy) relay(connection net.Conn) {
	defer proxy.workers.Done()
	defer connection.Close()
	if proxy.broken.Load() {
		return
	}
	upstream, err := net.DialTimeout("tcp", proxy.target, time.Second)
	if err != nil {
		proxy.t.Logf("real TCP relay dial error: %v", err)
		return
	}
	defer upstream.Close()
	proxy.mu.Lock()
	if proxy.broken.Load() {
		proxy.mu.Unlock()
		return
	}
	proxy.connections[connection], proxy.connections[upstream] = true, true
	proxy.mu.Unlock()
	defer proxy.forget(connection, upstream)
	var copies sync.WaitGroup
	copies.Add(1)
	go func() {
		defer copies.Done()
		if _, err := io.Copy(upstream, connection); err != nil {
			proxy.t.Logf("real TCP relay request error: %v", err)
		}
		upstream.Close()
	}()
	if _, err := io.Copy(connection, liveDelayedReader{Conn: upstream, proxy: proxy}); err != nil {
		proxy.t.Logf("real TCP relay response error: %v", err)
	}
	connection.Close()
	copies.Wait()
}

func (proxy *liveNetworkProxy) forget(a, b net.Conn) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	delete(proxy.connections, a)
	delete(proxy.connections, b)
}

func (proxy *liveNetworkProxy) disconnect() {
	proxy.broken.Store(true)
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	for connection := range proxy.connections {
		connection.Close()
	}
}

func (proxy *liveNetworkProxy) close() {
	proxy.listener.Close()
	proxy.disconnect()
	proxy.workers.Wait()
}

type liveDelayedReader struct {
	net.Conn
	proxy *liveNetworkProxy
}

func (reader liveDelayedReader) Read(buffer []byte) (int, error) {
	n, err := reader.Conn.Read(buffer)
	if n > 0 {
		if delay := reader.proxy.delay.Swap(0); delay > 0 {
			time.Sleep(time.Duration(delay))
		}
	}
	return n, err
}
