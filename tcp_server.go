package tcp_server

import (
	"bufio"
	"context"
	"crypto/tls"
	"log"
	"net"
	"sync"
)

// Client holds info about connection
type Client struct {
	sync.Mutex
	conn   net.Conn
	Server *Server
	ctx    context.Context
	Close  context.CancelFunc
}

// TCP server
type Server struct {
	sync.Mutex
	sync.WaitGroup
	address                  string // Address to open connection: localhost:9999
	config                   *tls.Config
	onNewClient              func(c *Client)
	onClientConnectionClosed func(c *Client)
	onNewMessage             func(c *Client, message string)
	messageTerminator        byte
	ctx                      context.Context
	Stop                     context.CancelFunc
}

var mctx = context.Background()

// Read client data from channel
func (c *Client) listen() {
	// Stopping our client
	go func() {
		<-c.ctx.Done()
		c.Server.Lock()
		c.conn.Close()
		c.Server.onClientConnectionClosed(c)
		c.Server.Unlock()
		c.Server.Done()
	}()
	reader := bufio.NewReader(c.conn)
	for {
		message, err := reader.ReadString(c.Server.messageTerminator)
		if err != nil {
			c.Close()
			return
		}
		c.Server.onNewMessage(c, message)
	}
}

// Send text message to client
func (c *Client) Send(message string) error {
	return c.SendBytes([]byte(message))
}

// Send bytes to client
func (c *Client) SendBytes(b []byte) error {
	_, err := c.conn.Write(b)
	if err != nil {
		c.Close()
	}
	return err
}

// Called right after server starts listening new client
func (s *Server) OnNewClient(callback func(c *Client)) {
	s.Lock()
	defer s.Unlock()
	s.onNewClient = callback
}

// Called right after connection closed
func (s *Server) OnClientConnectionClosed(callback func(c *Client)) {
	s.Lock()
	defer s.Unlock()
	s.onClientConnectionClosed = callback
}

// Called when Client receives new message
func (s *Server) OnNewMessage(callback func(c *Client, message string)) {
	s.Lock()
	defer s.Unlock()
	s.onNewMessage = callback
}

// Set message terminator
func (s *Server) MessageTerminator(terminator byte) {
	s.Lock()
	defer s.Unlock()
	s.messageTerminator = terminator
}

// Listen starts network server
func (s *Server) Listen() {
	s.Lock()
	var listener net.Listener
	var err error
	config := s.config
	address := s.address
	s.Unlock()
	if config == nil {
		listener, err = net.Listen("tcp", address)
	} else {
		listener, err = tls.Listen("tcp", address, config)
	}
	if err != nil {
		log.Fatal("Error starting TCP server.\r\n", err)
	}
	s.Lock()
	s.ctx, s.Stop = context.WithCancel(mctx)
	s.Add(1)
	s.Unlock()
	// Stopping our server.
	go func() {
		<-s.ctx.Done()
		listener.Close()
		s.Done()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			s.Stop()
			break
		}
		s.Lock()
		client := &Client{
			conn:   conn,
			Server: s,
		}
		client.ctx, client.Close = context.WithCancel(s.ctx)
		s.Add(1)
		s.onNewClient(client)
		s.Unlock()
		go client.listen()
	}
	s.Wait()
}

// Creates new tcp server instance
func New(address string) *Server {
	log.Println("Creating server with address", address)
	server := &Server{
		address:           address,
		messageTerminator: '\n',
	}

	server.OnNewClient(func(c *Client) {})
	server.OnNewMessage(func(c *Client, message string) {})
	server.OnClientConnectionClosed(func(c *Client) {})

	return server
}

func NewWithTLS(address, certFile, keyFile string) *Server {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Fatal("Error loading certificate files. Unable to create TCP server with TLS functionality.\r\n", err)
	}
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	server := New(address)
	server.config = config
	return server
}
