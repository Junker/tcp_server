# TCPServer
Package tcp_server created to help build TCP servers faster. Designed to work with text only.
Originally forked from https://github.com/firstrow/tcp_server and modified.

### Install package

``` bash
> go get github.com/tech10/tcp_server
```

### Usage:

NOTICE: `OnNewMessage` callback will receive new message only if it's ending with `\n`

``` go
package main

import "github.com/tech10/tcp_server"

func main() {
	server := tcp_server.New("localhost:9999")

	server.OnNewClient(func(c *tcp_server.Client) bool {
		// new client connected
		// lets send some message
		c.Send("Hello")
		//Now let's accept the connection.
		return true
	})
	server.OnNewMessage(func(c *tcp_server.Client, message string) {
		// new message received
		// Let's broadcast it to everyone.
		c.SendAll(message, nil)
	})
	server.OnClientConnectionClosed(func(c *tcp_server.Client, err error) {
		// connection with client lost
	})

	server.Listen()
}
```

# Contributing

To hack on this project:

1. Install as usual (`go get -u github.com/tech10/tcp_server`)
2. Create your feature branch (`git checkout -b my-new-feature`)
3. Ensure everything works and the tests pass (`go test`)
4. Commit your changes (`git commit -am 'Add some feature'`)

Contribute upstream:

1. Fork it on GitHub
2. Add your remote (`git remote add fork git@github.com:firstrow/tcp_server.git`)
3. Push to the branch (`git push fork my-new-feature`)
4. Create a new Pull Request on GitHub

Notice: Always use the original import path by installing with `go get`.
