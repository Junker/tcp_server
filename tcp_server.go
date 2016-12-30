package tcp_server

import (
	"bufio"
	"errors"
	"net"
	"sort"
	"strconv"
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
	onNewClient              func(c *Client) bool
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
	return stringFormatWithBS(message), err
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

func (c *Client) readprompt(prompt string) (string, bool) {
	if prompt != "" {
		err := c.Send(prompt)
		if err != nil {
			return "", true
		}
	}
	str, err := c.readln()
	if err != nil {
		return str, true
	}
	return str, false
}

// Read a line of data from a client, and prompt them what to enter.
func (c *Client) Read_prompt(prompt string) (string, bool) {
	prompt = strings.Trim(prompt, "\r\n")
	if prompt != "" {
		prompt += "\r\n"
	}
	str, aborted := c.readprompt(prompt + "Enter abort to cancel.")
	if aborted {
		return str, aborted
	}
	if strings.ToLower(str) == "abort" {
		aborted = true
		c.Send("Aborted.")
	}
	return str, aborted
}

// Get a yes or no prompt from the client.
func (c *Client) Read_confirm(prompt string) (bool, bool) {
	prompthead := ""
	prompt = strings.Trim(prompt, "\r\n")
	if prompt != "" {
		prompt += "\r\n"
	}
	var res bool
	var aborted bool
	var answer string
loop:
	for {
		answer, aborted = c.readprompt(prompthead + prompt + "Enter yes, no, or abort to cancel.")
		if aborted {
			return res, aborted
		}
		switch strings.ToLower(answer) {
		case "":
			prompthead = "An empty value isn't supported.\r\n"
			continue loop
		case "abort":
			c.Send("Aborted.")
			aborted = true
			break loop
		case "y", "yes":
			res = true
			aborted = false
			break loop
		case "n", "no":
			res = false
			aborted = false
			break loop
		default:
			prompthead = "The entry " + answer + " is unsupported.\r\n"
			continue loop
		}
	}
	return res, aborted
}

// Give a client an option to select from a menu.
func (c *Client) Read_menu(prompt string, menu []string) (int, bool) {
	prompt = strings.Trim(prompt, "\r\n")
	if prompt != "" {
		prompt += "\r\n"
	}
	if len(menu) == 0 {
		return -1, true
	}
	menuselect := []string{}
	for i, string := range menu {
		if string == "" {
			continue
		}
		index := strconv.Itoa(i + 1)
		menuselect = append(menuselect, "["+index+"]: "+string)
	}
	menumsg := strings.Join(menuselect, "\r\n") + "\r\n"
	rangemin := 1
	rangemax := len(menu)
	abortmsg := "Enter abort to cancel."
	prompthead := ""
	var res int
	var aborted bool
	var answer string
	for {
		answer, aborted = c.readprompt(prompthead + prompt + menumsg + abortmsg)
		if !aborted {
			return -1, aborted
		}
		if strings.ToLower(answer) == "abort" {
			c.Send("Aborted.")
			return -1, true
		}
		if answer == "" {
			prompthead = "An empty value isn't accepted.\r\n"
			continue
		}
		int, err := strconv.Atoi(answer)
		if err != nil || int < rangemin || int > rangemax {
			prompthead = "Invalid selection.\r\n"
			continue
		}
		aborted = false
		res = int - 1
		break
	}
	return res, aborted
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
	if message == "\r\n" {
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

// Send text message to all authorized clients.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (c *Client) SendAllAuthorized(message string) (int, error) {
	count := 0
	if message == "" {
		return count, errors.New("empty string invalid")
	}
	clients := c.server.clients_sorted()
	if len(clients) == 0 {
		return count, errors.New("no clients available")
	}
	for _, sc := range clients {
		sc.Lock()
		if !sc.authorized {
			sc.Unlock()
			continue
		}
		sc.Unlock()
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
	s := c.server
	if c.connected {
		err = c.conn.Close()
		c.connected = false
		if c.authorized {
			c.authorized = false
			c.Unlock()
			s.onClientConnectionClosed(c, err)
		} else {
			c.Unlock()
		}
		s.remove(c.id)
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
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.started = true
	s.listener = &listener
	s.wg.Add(1)
	go s.process()
	return err
}

// Shut down the server and disconnect all connected clients.
func (s *Server) Stop() {
	s.Lock()
	defer s.Unlock()
	(*s.listener).Close()
	for _, c := range s.clients {
		if c != nil {
			s.Unlock()
			c.close()
			s.Lock()
		}
	}
}

// Process accepting clients until the server is shut down.
func (s *Server) process() {
	defer s.wg.Done()
	s.accept()
}

// Wait for server processing to complete. This will happen when all clients are disconnected and the server is shut down.
func (s *Server) Wait() {
	s.wg.Wait()
}

// Accept client connections.
func (s *Server) accept() {
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
		s.wg.Add(1)
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
	return s.clients_sorted()
}

// Sends a message to all connected clients.
func (s *Server) SendAll(message string) (int, error) {
	clients := server.clients_sorted()
	count := 0
	if message == "" {
		return count, errors.New("empty message not allowed")
	}
	if len(clients) == 0 {
		return count, errors.New("no clients connected")
	}
	for _, c := range clients {
		if c.Send(message) == nil {
			count++
		}
	}
	if count == 0 {
		return count, errors.New("message failed to send to connected clients")
	}
	return count, nil
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
