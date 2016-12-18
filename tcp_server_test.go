package tcp_server

import (
	"bufio"
	. "github.com/smartystreets/goconvey/convey"
	"net"
	"testing"
	"time"
)

func buildTestServer() *server {
	return New("localhost:9999")
}

func Test_accepting_new_client_callback(t *testing.T) {
	server := buildTestServer()

	var messageReceived bool
	var messageText string
	var messageTextReceived string
	messageTest := "testing"

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
	})
	go server.Listen()

	// Wait for server
	// If test fails - increase this value
	time.Sleep(10 * time.Millisecond)

	conn, err := net.Dial("tcp", "localhost:9999")
	if err != nil {
		t.Fatal("Failed to connect to test server")
	}
	conn.Write([]byte(messageTest + "\n"))
	messageTextReceived, err = bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatal("Failed to receive message.\r\n" + err.Error())
	}
	conn.Close()

	// Wait for server
	time.Sleep(10 * time.Millisecond)

	Convey("Messages should be equal", t, func() {
		So(messageText, ShouldEqual, messageTest)
		So(messageTextReceived, ShouldEqual, messageTest+"\r\n")
	})
	Convey("It should receive new client callback", t, func() {
		So(newClient, ShouldEqual, true)
	})
	Convey("It should receive message callback", t, func() {
		So(messageReceived, ShouldEqual, true)
	})
	Convey("It should receive connection closed callback", t, func() {
		So(connectionClosed, ShouldEqual, true)
	})
}
