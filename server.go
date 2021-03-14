package tcp_server

import (
	"bufio"
	"crypto/tls"
	"errors"
	"net"
	"sort"
	"sync"
)

// server instance.
//It should not be necessary to interact with any of these variables directly.
type Server struct {
	sync.Mutex
	wg                       sync.WaitGroup
	clients                  map[float64]*Client
	address                  string
	listener                 net.Listener
	config                   *tls.Config
	started                  bool
	maxid                    float64
	onNewClient              func(c *Client) bool
	onClientConnectionClosed func(c *Client, err error)
	onNewMessage             func(c *Client, message string)
}

// Called when a client connection is received, and before data is received by the client in the background.
// To accept a connection, this function must return true.
func (s *Server) OnNewClient(callback func(c *Client) bool) {
	s.Lock()
	s.onNewClient = callback
	s.Unlock()
}

// Called after Client is disconnected.
func (s *Server) OnClientConnectionClosed(callback func(c *Client, err error)) {
	s.Lock()
	s.onClientConnectionClosed = callback
	s.Unlock()
}

// Called when Client receives new message
func (s *Server) OnNewMessage(callback func(c *Client, message string)) {
	s.Lock()
	s.onNewMessage = callback
	s.Unlock()
}

// Start server
func (s *Server) Start() error {
	s.Lock()
	defer s.Unlock()
	if s.started {
		return errors.New("already started")
	}
	var err error
	var listener net.Listener
	if s.config == nil {
		listener, err = net.Listen("tcp", s.address)
	} else {
		listener, err = tls.Listen("tcp", s.address, s.config)
	}
	if err != nil {
		return err
	}
	s.started = true
	s.listener = listener
	s.wg.Add(1)
	go s.process()
	return err
}

// Shut down the server and disconnect all connected clients.
func (s *Server) Stop() {
	s.Lock()
	defer s.Unlock()
	err := s.listener.Close()
	if err != nil {
		return
	}
	for _, c := range s.clients {
		if c != nil {
			s.Unlock()
			c.close()
			s.Lock()
		}
	}
}

func (s *Server) process() {
	defer s.wg.Done()
	s.accept()
}

// Wait for server processing to complete. This will happen when all clients are disconnected and the server is shut down.
func (s *Server) Wait() {
	s.wg.Wait()
}

func (s *Server) accept() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		client := &Client{
			conn:   conn,
			ip:     ip,
			r:      bufio.NewReader(conn),
			pmsg:   make(chan string),
			w:      bufio.NewWriter(conn),
			server: s,
		}
		s.wg.Add(1)
		go s.add(client)
	}
}

func (s *Server) clientsSorted() []*Client {
	clients := []*Client{}
	ids := []float64{}
	s.Lock()
	defer s.Unlock()
	if len(s.clients) == 0 {
		return clients
	}
	for id, c := range s.clients {
		if c != nil {
			ids = append(ids, id)
		}
	}
	sort.Float64s(ids)
	for _, id := range ids {
		clients = append(clients, s.clients[id])
	}
	return clients
}

// Returns the clients in there connection order.
func (s *Server) Clients() []*Client {
	return s.clientsSorted()
}

func (s *Server) add(c *Client) {
	s.Lock()
	s.clients[s.maxid] = c
	c.id = s.maxid
	s.maxid++
	c.connected = true
	s.Unlock()
	if !s.onNewClient(c) {
		c.close()
		return
	}
	go c.listen()
}

func (s *Server) remove(cid float64) {
	s.Lock()
	_, exists := s.clients[cid]
	if exists {
		delete(s.clients, cid)
	}
	s.Unlock()
	s.wg.Done()
}

// Send text message to all clients accept the client excluded.
// Set excluded to nill to send to all clients.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (s *Server) SendAll(message string, excluded *Client) (int, error) {
	count := 0
	if message == "" {
		return count, errors.New("empty string invalid")
	}
	clients := s.clientsSorted()
	if len(clients) == 0 {
		return count, errors.New("no clients available")
	}
	for _, sc := range clients {
		if excluded != nil && sc == excluded {
			continue
		}
		sent := sc.Send(message)
		if sent {
			count++
		}
	}
	if count == 0 {
		return count, errors.New("sent to no clients")
	}
	return count, nil
}

func (s *Server) sendAuthorized(message string, excluded *Client, authorized bool) (int, error) {
	count := 0
	if message == "" {
		return count, errors.New("empty string invalid")
	}
	clients := s.clientsSorted()
	if len(clients) == 0 {
		return count, errors.New("no clients available")
	}
	for _, sc := range clients {
		sc.Lock()
		if sc.authorized != authorized {
			sc.Unlock()
			continue
		}
		sc.Unlock()
		if excluded != nil && sc == excluded {
			continue
		}
		sent := sc.Send(message)
		if sent {
			count++
		}
	}
	if count == 0 {
		return count, errors.New("sent to no clients")
	}
	return count, nil
}

// Send text message to all authorized clients, except the excluded client.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (s *Server) SendAllAuthorized(message string, excluded *Client) (int, error) {
	return s.sendAuthorized(message, excluded, true)
}

// Send text message to all unauthorized clients, except the excluded client.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (s *Server) SendAllUnauthorized(message string, excluded *Client) (int, error) {
	return s.sendAuthorized(message, excluded, false)
}

// Creates new tcp server instance
func New(address string) *Server {
	server := &Server{
		address: address,
		config:  nil,
		clients: make(map[float64]*Client),
		maxid:   1,
	}

	server.OnNewClient(func(c *Client) bool {
		return false
	})
	server.OnNewMessage(func(c *Client, message string) {})
	server.OnClientConnectionClosed(func(c *Client, err error) {})

	return server
}

func NewWithTLS(address string, certFile string, keyFile string) *Server {
	cert, _ := tls.LoadX509KeyPair(certFile, keyFile)
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	server := New(address)
	server.config = config
	return server
}
