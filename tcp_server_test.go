package tcp_server

import (
	"net"
	"runtime"
	"sync"
	"testing"
	"time"
)

const test_time_ms = 10

var server *Server

func Test_accepting_new_client_callback(t *testing.T) {
	server = New("localhost:9999")

	var wg sync.WaitGroup
	wg.Add(3)

	var messageText string

	server.OnNewClient(func(c *Client) {
		wg.Done()
	})
	server.OnNewMessage(func(c *Client, message string) {
		wg.Done()
		messageText = message
	})
	server.OnClientConnectionClosed(func(c *Client) {
		wg.Done()
	})
	go server.Listen()

	// Wait for server
	// If test fails - increase this value
	time.Sleep(test_time_ms * time.Millisecond)

	conn, err := net.Dial("tcp", "localhost:9999")
	if err != nil {
		t.Fatal("Failed to connect to test server")
	}
	_, err = conn.Write([]byte("Test message\n"))
	if err != nil {
		t.Fatal("Failed to send test message.")
	}
	conn.Close()

	wg.Wait()

	if messageText != "Test message\n" {
		t.Error("received wrong message")
	}
}

func Test_accepting_new_client_callback_different_terminator(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(3)

	var messageText string

	server.OnNewClient(func(c *Client) {
		wg.Done()
	})
	server.OnNewMessage(func(c *Client, message string) {
		wg.Done()
		messageText = message
	})
	server.OnClientConnectionClosed(func(c *Client) {
		wg.Done()
	})
	server.MessageTerminator('\u0000')

	conn, err := net.Dial("tcp", "localhost:9999")
	if err != nil {
		t.Fatal("Failed to connect to test server")
	}
	_, err = conn.Write([]byte("Test message\u0000"))
	if err != nil {
		t.Fatal("Failed to send test message.")
	}
	conn.Close()

	wg.Wait()

	if messageText != "Test message\u0000" {
		t.Error("received wrong message")
	}
}

func Test_concurrency(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU() * 4)
	var client *Client
	var wg sync.WaitGroup
	wg.Add(1)

	server.OnNewClient(func(c *Client) {
		client = c
		wg.Done()
	})
	server.OnNewMessage(func(c *Client, message string) {
	})
	server.OnClientConnectionClosed(func(c *Client) {
	})

	conn, err := net.Dial("tcp", "localhost:9999")
	if err != nil {
		t.Fatal("Failed to connect to test server")
	}
	wg.Wait()

	for o := 0; o < 10; o++ {
		for i := 0; i < 10; i++ {
			go func() {
				runtime.Gosched()
				go func() {
					_ = client.SendBytes([]byte("Test.\n"))
				}()
				runtime.Gosched()
				_ = client.SendBytes([]byte("Test.\n"))
			}()
		}
	}

	conn.Close()
	server.Stop()

	// Wait for server
	// If test fails - increase this value
	time.Sleep(test_time_ms * time.Millisecond)
}

func Test_server_stop(t *testing.T) {

	server.Stop()

	// Wait for server
	// If test fails - increase this value
	time.Sleep(test_time_ms * time.Millisecond)

	_, err := net.Dial("tcp", "localhost:9999")
	if err == nil {
		t.Fatal("Connected to test server, which should be stopped.")
	}
}
