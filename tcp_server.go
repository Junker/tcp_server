package tcp_server

import (
	"bufio"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
)

// Client holds info about connection
type Client struct {
	//Make thread safe from data races.
	sync.Mutex
	conn      net.Conn
	connected bool
	ip        string
	r         *bufio.Reader
	w         *bufio.Writer
	id        float64
	server    *server
}

// TCP server
type server struct {
	//Make thread safe from data races.
	sync.Mutex
	clients                  map[float64]*Client
	address                  string  // Address to open
	maxid                    float64 // Maximum available client ID
	onNewClientCallback      func(c *Client) bool
	onClientConnectionClosed func(c *Client, err error)
	onNewMessage             func(c *Client, message string)
}

// Read client data
func (c *Client) listen() {
	c.Lock()
	r := c.r
	c.Unlock()
	for {
		message, err := r.ReadString('\n')
		if err != nil {
			c.close()
			return
		}
		c.server.onNewMessage(c, strings.Trim(message, "\r\n"))
	}
}

// Get clients IP address
func (c *Client) IP() string {
	c.Lock()
	defer c.Unlock()
	return c.ip
}

// Send text message to client
func (c *Client) Send(message string) error {
	message = strings.Trim(message, "\r\n") + "\r\n"
	if message == "" {
		return errors.New("empty string invalid")
	}
	c.Lock()
	c.w.WriteString(message)
	err := c.w.Flush()
	c.Unlock()
	if err != nil {
		c.close()
	}
	return err
}

// Send text message to all clients accept the client excluded. Set to nill to send to all clients.
func (c *Client) SendAll(message string, excluded *Client) (int, error) {
	count := 0
	if message == "" {
		return count, errors.New("empty string invalid")
	}
	clients := c.server.clients_sorted()
	if len(clients) == 0 {
		return count, errors.New("no clients available")
	}
	for _, sc := range clients {
		if excluded != nil && sc == excluded {
			continue
		}
		err := sc.Send(message)
		if err == nil {
			count++
		}
	}
	if count == 0 {
		return count, errors.New("sent to no clients")
	}
	return count, nil
}

// Returns the client ID
func (c *Client) ID() int64 {
	c.Lock()
	defer c.Unlock()
	return int64(c.id)
}

func (c *Client) close() error {
	var err error
	c.Lock()
	if c.connected {
		err = c.conn.Close()
		c.connected = false
		c.server.remove(c.id)
		c.server.onClientConnectionClosed(c, err)
	} else {
		err = errors.New("already disconnected")
	}
	c.Unlock()
	return err
}

// Closes an open client connection.
func (c *Client) Close() error {
	return c.close()
}

// Called right before server starts listening to a new client instance
func (s *server) OnNewClient(callback func(c *Client) bool) {
	s.onNewClientCallback = callback
}

// Called right after connection closed
func (s *server) OnClientConnectionClosed(callback func(c *Client, err error)) {
	s.onClientConnectionClosed = callback
}

// Called when Client receives new message
func (s *server) OnNewMessage(callback func(c *Client, message string)) {
	s.onNewMessage = callback
}

// Start network server
func (s *server) Listen() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		conn, _ := listener.Accept()
		ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		client := &Client{
			conn:   conn,
			ip:     ip,
			r:      bufio.NewReader(conn),
			w:      bufio.NewWriter(conn),
			server: s,
		}
		s.add(client)
		go client.listen()
	}
	return nil
}

// Sorts client connections by increasing ID.

func (s *server) clients_sorted() []*Client {
	clients := []*Client{}
	ids := []float64{}
	s.Lock()
	defer s.Unlock()
	if len(s.clients) == 0 {
		return clients
	}
	for id := range s.clients {
		ids = append(ids, id)
	}
	sort.Float64s(ids)
	for _, id := range ids {
		clients = append(clients, s.clients[id])
	}
	return clients
}

// Returns the clients in there connection order.
func (s *server) Clients() []*Client {
	return s.clients_sorted()
}

func (s *server) add(c *Client) {
	if !s.onNewClientCallback(c) {
		c.conn.Close()
		return
	}
	s.Lock()
	s.clients[s.maxid] = c
	c.id = s.maxid
	c.connected = true
	s.maxid++
	s.Unlock()
}

func (s *server) remove(cid float64) {
	s.Lock()
	defer s.Unlock()
	_, exists := s.clients[cid]
	if exists {
		delete(s.clients, cid)
	}
}

// Creates new tcp server instance
func New(address string) *server {
	server := &server{
		address: address,
		clients: make(map[float64]*Client, 0),
		maxid:   1,
	}

	server.OnNewClient(func(c *Client) bool {
		return false
	})
	server.OnNewMessage(func(c *Client, message string) {})
	server.OnClientConnectionClosed(func(c *Client, err error) {})

	return server
}
