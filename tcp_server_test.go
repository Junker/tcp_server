package tcp_server

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

const addr = "127.0.0.1:9999"

var server *Server

var l sync.Mutex

var messageReceived bool
var messageText string
var messageTextReceived string
var newClient bool
var connectionClosed bool

var err error

func TestMain(m *testing.M) {
	server = New(addr)
	err = server.Start()
	if err != nil {
		fmt.Println("Unable to run. Server not started.", err)
		os.Exit(4)
	}

	serverFuncsSet()

	res := m.Run()
	server.Stop()
	server.Wait()
	os.Exit(res)
}

func serverFuncsSet() {
	l.Lock()
	server.OnNewClient(func(c *Client) bool {
		l.Lock()
		newClient = true
		l.Unlock()
		return true
	})
	server.OnNewMessage(func(c *Client, message string) {
		l.Lock()
		messageReceived = true
		messageText = message
		l.Unlock()
		c.Send(message + "\r\r\n\n")
		c.Close()
	})
	server.OnClientConnectionClosed(func(c *Client, err error) {
		l.Lock()
		connectionClosed = true
		l.Unlock()
	})
	l.Unlock()
}

func Test_accepting_new_client_callback(t *testing.T) {

	l.Lock()
	newClient = false
	messageReceived = false
	messageText = ""
	connectionClosed = false
	l.Unlock()

	messageTest := "This is going to be a test of the receiving and sending of messages in both directions."

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal("Failed to connect to test server. Couldn't test client messages being accepted and processed.", err)
	}
	conn.Write([]byte(messageTest + "\n"))
	messageTextReceived, err = bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal("Failed to receive message for processing.", err)
	}
	conn.Close()

	//Wait for the server to finish calling our functions.
	time.Sleep(time.Millisecond * 10)

	l.Lock()

	if messageText != messageTest {
		t.Error("Message received from callback function and test message aren't equal. Neither should have newline characters.\r\nReceived \"" + messageText + "\"\r\nTest message: \"" + messageTest + "\"\r\n")
	}

	if messageTextReceived != messageTest+"\r\n" {
		t.Error("Message received from client bufio reader and test message aren't equal. The message received from the reader should have newline characters.\r\nReceived from bufio reader \"" + messageTextReceived + "\"\r\nTest message: \"" + messageTest + "\"\r\n")
	}

	if !newClient {
		t.Error("onNewClientCallback function wasn't called, and should have been. Client connection wasn't accepted.")
	}
	if !messageReceived {
		t.Error("onNewMessageCallback function wasn't called, and should have been. Client message wasn't processed.")
	}
	if !connectionClosed {
		t.Error("onClientConnectionClosedCallback function wasn't called, and should have been. Client connection wasn't accepted.")
	}
	l.Unlock()
}

func Test_rejecting_new_client_callback(t *testing.T) {

	l.Lock()
	newClient = false
	messageReceived = false
	messageText = ""
	connectionClosed = false
	l.Unlock()

	messageTest := "This is going to be a test of the server rejecting our connection. This message shouldn't be received.\n"

	l.Lock()
	server.OnNewClient(func(c *Client) bool {
		l.Lock()
		newClient = true
		l.Unlock()
		return false
	})
	l.Unlock()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal("Failed to connect to test server. Couldn't test client connection rejection.", err)
	}

	conn.Write([]byte(messageTest))
	conn.Close()
	//Wait for the server to finish executing our functions.
	time.Sleep(time.Millisecond * 10)

	l.Lock()

	if !newClient {
		t.Error("onNewClientCallback function wasn't called, and should have been. Client connection couldn't be rejected.")
	}
	if messageReceived {
		t.Error("onNewMessageCallback function was called, and shouldn't have been. Client connection wasn't rejected.")
	}
	if connectionClosed {
		t.Error("onClientConnectionClosedCallback function was called, and shouldn't have been. Client connection wasn't rejected.")
	}
	l.Unlock()
	serverFuncsSet()
}

func Benchmark_client_connections_disconnections_messages(b *testing.B) {

	msg := []byte("test\n")

	for i := 0; i < b.N; i++ {
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			b.Fatal("Failed to connect to test server. Unable to run benchmark.", err)
		}
		conn.Write(msg)
		conn.Close()
	}

}

func Benchmark_parallel_client_connections_disconnections_messages(b *testing.B) {

	msg := []byte("test\n")

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				b.Fatal("Couldn't connect to test server. Unable to complete parallel execution test.", err)
			}
			conn.Write(msg)
			conn.Close()
		}
	})

}
