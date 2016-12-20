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
		// lets send a message
		c.Send("Hello\n")
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

	err := server.Start()
	if err != nil {
		return
		}
	server.Wait()
}
```

# Contributing

To contribute:

1. Install as usual (`go get -u github.com/tech10/tcp_server`)
2. Create your feature branch (`git checkout -b my-new-feature`)
3. Ensure everything works and the tests pass (`go test`)
4. Make sure there aren't any data races (`go test -race`)
5. Commit your changes (`git commit -am 'Add some feature'`)

Contribute upstream:

1. Fork it on GitHub
2. Add your remote (`git remote add fork git@github.com:tech10/tcp_server.git`)
3. Complete all the tests stated above.
4. Push to the branch (`git push fork my-new-feature`)
5. Create a new Pull Request on GitHub

Notice: Always use the original import path by installing with `go get`.
