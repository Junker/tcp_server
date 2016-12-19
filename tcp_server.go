package tcp_server

import (
	"bufio"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
)

// Client holds info about a connection to the server.
// It should never be necessary to interact with any variables within this type directly.
type Client struct {
	sync.Mutex
	conn       net.Conn
	connected  bool
	authorized bool
	ip         string
	r          *bufio.Reader
	w          *bufio.Writer
	id         float64
	server     *Server
}

// TCP server instance.
//It should not be necessary to interact with any of these variables directly.
type Server struct {
	sync.Mutex
	wg                       sync.WaitGroup
	clients                  map[float64]*Client
	address                  string
	listener                 *net.Listener
	started                  bool
	maxid                    float64
	onNewClientCallback      func(c *Client) bool
	onClientConnectionClosed func(c *Client, err error)
	onNewMessage             func(c *Client, message string)
}

// Read a single line of data from the client without calling the callback function.
func (c *Client) Readln() (string, error) {
	return c.readln()
}

func (c *Client) readln() (string, error) {
	c.Lock()
	if !c.connected {
		c.Unlock()
		return "", errors.New("client not connected")
	}
	r := c.r
	c.Unlock()
	message, err := r.ReadString('\n')
	if err != nil {
		c.close()
		return "", err
	}
	return strings.Trim(message, "\r\n"), err
}

// Read client data until disconnected
func (c *Client) listen() {
	c.Lock()
	c.authorized = true
	c.Unlock()
	for {
		message, err := c.readln()
		if err != nil {
			return
		}
		c.server.onNewMessage(c, message)
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

// Send text message to all clients accept the client excluded.
// Set excluded to nill to send to all clients.
// Returns the number of clients data was sent to, and an error if the number is 0.
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

// Gets the client ID.
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
		if c.authorized {
			c.Unlock()
			c.server.onClientConnectionClosed(c, err)
		} else {
			c.Unlock()
		}
		c.server.remove(c.id)
		c.server.wg.Done()
	} else {
		err = errors.New("already disconnected")
		c.Unlock()
	}
	return err
}

// Closes an open client connection, and calls the OnConnectionClose() callback function.
func (c *Client) Close() error {
	return c.close()
}

// Called when a client connection is received, and before data is received by the client.
// To accept a connection, this function must return true.
func (s *Server) OnNewClient(callback func(c *Client) bool) {
	s.onNewClientCallback = callback
}

// Called after Client is disconnected.
func (s *Server) OnClientConnectionClosed(callback func(c *Client, err error)) {
	s.onClientConnectionClosed = callback
}

// Called when Client receives new message
func (s *Server) OnNewMessage(callback func(c *Client, message string)) {
	s.onNewMessage = callback
}

// Start server
func (s *Server) Start() error {
	s.Lock()
	defer s.Unlock()
	if s.started {
		return errors.New("already started")
	}
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.started = true
	s.listener = &listener
	return nil
}

// Shut down the server and disconnect all connected clients.
func (s *Server) Stop() {
	s.Lock()
	clients := s.clients
	(*s.listener).Close()
	s.Unlock()
	for _, c := range clients {
		c.close()
	}
}

// Process accepting clients until the server is shut down.
func (s *Server) Process() {
	s.wg.Add(1)
	go s.accept()
	s.wg.Wait()
}

// Accept client connections.
func (s *Server) accept() {
	defer s.wg.Done()
	s.Lock()
	listener := s.listener
	s.Unlock()
	for {
		conn, err := (*listener).Accept()
		if err != nil {
			return
		}
		ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		client := &Client{
			conn:   conn,
			ip:     ip,
			r:      bufio.NewReader(conn),
			w:      bufio.NewWriter(conn),
			server: s,
		}
		go s.add(client)
	}
}

// Sorts client connections by increasing ID.
func (s *Server) clients_sorted() []*Client {
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
func (s *Server) Clients() []*Client {
	return s.clients_sorted()
}

func (s *Server) add(c *Client) {
	s.wg.Add(1)
	s.Lock()
	s.clients[s.maxid] = c
	c.id = s.maxid
	s.maxid++
	c.connected = true
	s.Unlock()
	if !s.onNewClientCallback(c) {
		c.close()
		return
	}
	go c.listen()
}

func (s *Server) remove(cid float64) {
	s.Lock()
	defer s.Unlock()
	_, exists := s.clients[cid]
	if exists {
		delete(s.clients, cid)
	}
}

// Creates new tcp server instance
func New(address string) *Server {
	server := &Server{
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
