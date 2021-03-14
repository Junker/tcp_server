package tcp_server

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// Client holds info about a connection to the server.
// It should never be necessary to interact with any variables within this type directly.
type Client struct {
	sync.Mutex
	conn            net.Conn
	connected       bool
	authorized      bool
	listening       bool
	callbackRunning bool
	ip              string
	host            string
	hostCached      bool
	r               *bufio.Reader
	p               sync.Mutex
	pmsg            chan string
	prompt          bool
	w               *bufio.Writer
	id              float64
	server          *Server
	db              map[string]interface{}
	dbl             sync.Mutex
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
	c.Unlock()
	var message string
	var err error
	message, err = c.r.ReadString('\n')
	if err != nil {
		c.close()
		return "", err
	}
	return stringFormatWithBS(message), err
}

func (c *Client) listen() {
	c.Lock()
	c.authorized = true
	c.listening = true
	c.Unlock()
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "Recovered from", r)
			fmt.Fprintln(os.Stderr, debug.Stack())
			c.close()
		}
		c.Lock()
		c.listening = false
		c.callbackRunning = false
		c.Unlock()
	}()
	for {
		message, err := c.readln()
		if err != nil {
			return
		}
		c.Lock()
		prompt := c.prompt
		c.Unlock()
		if prompt {
			c.pmsg <- message
			continue
		}
		c.Lock()
		c.listening = false
		c.callbackRunning = true
		c.Unlock()
		c.server.onNewMessage(c, message)
		c.Lock()
		c.listening = true
		c.callbackRunning = false
		c.Unlock()
	}
}

// You can call this method to stop the function executed when messages are received.
// Exiting the goroutine yourself will cause the program to stop sending messages to your function for receiving client messages, but exiting this function will ensure messages are still received, while at the same time, exiting the goroutine.
func (c *Client) Stop() {
	c.Lock()
	listener := c.callbackRunning
	c.Unlock()
	if !listener {
		return
	}
	defer func() {
		go c.listen()
	}()
	runtime.Goexit()
}

func (c *Client) readprompt(prompt string) (string, bool) {
	c.p.Lock()
	defer c.p.Unlock()
	c.Lock()
	c.prompt = true
	listening := c.listening
	c.Unlock()
	defer func() {
		c.Lock()
		c.prompt = false
		c.Unlock()
	}()
	if prompt != "" {
		sent := c.Send(prompt)
		if !sent {
			return "", true
		}
	}
	var str string
	var err error
	var ok bool
	if !listening {
		str, err = c.readln()
	} else {
		str, ok = <-c.pmsg
		if !ok {
			err = errors.New("prompt channel closed")
		}
	}
	if err != nil {
		return str, true
	}
	return str, false
}

// Read a line of data from a client, and prompt them what to enter.
func (c *Client) ReadPrompt(prompt string) (string, bool) {
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
func (c *Client) ReadPromptConfirm(prompt string) (bool, bool) {
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
func (c *Client) ReadPromptMenu(prompt string, menu []string) (int, bool) {
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
	res := -1
	var aborted bool
	var answer string
	for {
		res = -1
		answer, aborted = c.readprompt(prompthead + prompt + menumsg + abortmsg)
		if !aborted {
			return res, aborted
		}
		if strings.ToLower(answer) == "abort" {
			c.Send("Aborted.")
			break
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

// Get clients hostname by doing an RDNS lookup on the IP address.
func (c *Client) Host() string {
	c.Lock()
	defer c.Unlock()
	if c.hostCached {
		return c.host
	}
	ip := c.ip
	c.Unlock()
	hosts, err := net.LookupAddr(ip)
	if err != nil {
		c.Lock()
		return ""
	}
	if len(hosts) == 1 {
		c.Lock()
		c.host = hostCheck(hosts[0])
		c.hostCached = true
		return c.host
	} else {
		c.Lock()
		for i, host := range hosts {
			if i+1 != len(hosts) {
				c.host += hostCheck(host) + ", "
			} else {
				c.host += hostCheck(host)
			}
		}
		c.hostCached = true
	}
	return c.host
}

// Send text message to client
func (c *Client) Send(message string) bool {
	message = strings.Trim(message, "\r\n") + "\r\n"
	if message == "\r\n" {
		return false
	}
	c.Lock()
	_, wErr := c.w.WriteString(message)
	err := c.w.Flush()
	c.Unlock()
	if err != nil || wErr != nil {
		c.close()
	}
	return true
}

// Send text message to all clients accept the client excluded.
// Set excluded to nill to send to all clients.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (c *Client) SendAll(message string, excluded *Client) (int, error) {
	return c.server.SendAll(message, excluded)
}

// Send text message to all authorized clients, except the excluded client.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (c *Client) SendAllAuthorized(message string, excluded *Client) (int, error) {
	return c.server.SendAllAuthorized(message, excluded)
}

// Send text message to all unauthorized clients, except the excluded client.
// Returns the number of clients data was sent to, and an error if the number is 0.
func (c *Client) SendAllUnauthorized(message string, excluded *Client) (int, error) {
	return c.server.SendAllUnauthorized(message, excluded)
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
		close(c.pmsg)
		if c.authorized {
			c.authorized = false
			c.Unlock()
			s.onClientConnectionClosed(c, err)
			c.Lock()
		}
		c.Unlock()
		s.remove(c.id)
		c.DataClear()
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

// Returns the clients server instance.
func (c *Client) Server() *Server {
	c.Lock()
	defer c.Unlock()
	return c.server
}

// Set a data value.
func (c *Client) DataSet(key string, value interface{}) {
	c.dbl.Lock()
	defer c.dbl.Unlock()
	if c.db == nil {
		c.db = make(map[string]interface{})
	}
	c.db[key] = value
}

// Reads data from the database.
// To correctly use this in your programs,
// you will need to call it with a type assertion.
// For example,
// time, found := c.DataGet("time").(time.Time)
// if !found {
// do something here.
// } else {
// return time
// }

func (c *Client) DataGet(key string) interface{} {
	c.dbl.Lock()
	defer c.dbl.Unlock()
	if _, exists := c.db[key]; !exists {
		return nil
	}
	return c.db[key]
}

// Clears the client database.
func (c *Client) DataClear() {
	c.dbl.Lock()
	defer c.dbl.Unlock()
	c.db = nil
}

func stringFormatWithBS(str string) string {
	if str == "" {
		return ""
	}
	ts := ""
	for _, chr := range str {
		if chr == 8 {
			if ts != "" && len(ts) > 1 {
				ts = ts[:len(ts)-1]
				continue
			} else if len(ts) == 1 {
				ts = ""
				continue
			}
			continue
		}
		if chr < 32 || chr > 126 {
			continue
		}
		ts += string(chr)
	}
	return ts
}

func hostCheck(host string) string {
	return strings.TrimSuffix(host, ".")
}
