package tcp_server

import "strings"

func host_check(host string) string {
	return strings.TrimSuffix(host, ".")
}
