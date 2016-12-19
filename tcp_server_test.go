package tcp_server

import (
	"bufio"
	"net"
	"testing"
)

func buildTestServer() *Server {
	return New("localhost:9999")
}

func Test_accepting_new_client_callback(t *testing.T) {
	server := buildTestServer()

	var messageReceived bool
	var messageText string
	var messageTextReceived string
	messageTest := "This is going to be a test of the receiving and sending of messages in both directions."

	var newClient bool
	var connectionClosed bool

	var err error

	server.OnNewClient(func(c *Client) bool {

		newClient = true
		return true
	})
	server.OnNewMessage(func(c *Client, message string) {
		messageReceived = true
		messageText = message
		c.Send(message + "\r\r\n\n")
	})
	server.OnClientConnectionClosed(func(c *Client, err error) {
		connectionClosed = true
		c.server.Stop()
	})
	err = server.Start()
	if err != nil {
		t.Fatal("Failed to start server.\r\n" + err.Error())
	}

	go func() {
		conn, err := net.Dial("tcp", "localhost:9999")
		if err != nil {
			t.Fatal("Failed to connect to test server.")
		}
		conn.Write([]byte(messageTest + "\n"))
		messageTextReceived, err = bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			t.Fatal("Failed to receive message.\r\n" + err.Error())
		}
		conn.Close()
	}()
	// Wait for server
	server.Process()

	// Server is now shut down.
	if messageText != messageTest {
		t.Fatal("Message received from callback function and test message aren't equal. Neither should have newline characters.\r\nReceived \"" + messageText + "\"\r\nTest message: \"" + messageTest + "\"\r\n")
	}

	if messageTextReceived != messageTest+"\r\n" {
		t.Fatal("Message received from client bufio reader and test message aren't equal. The message received from the reader should have newline characters.\r\nReceived from bufio reader \"" + messageTextReceived + "\"\r\nTest message: \"" + messageTest + "\"\r\n")
	}

	if !newClient {
		t.Fatal("onNewClientCallback function wasn't called, and should have been.")
	}
	if !messageReceived {
		t.Fatal("onNewMessageCallback function wasn't called, and should have been.")
	}
	if !connectionClosed {
		t.Fatal("onClientConnectionClosedCallback function wasn't called, and should have been.")
	}

}
