package pac

import (
	"fmt"
	"net"
	"strings"
)

// Proxies is a list of proxies as returned from FindProxyForURL.
// If the string is empty, no proxies should be used.
// The string can contain any number of the following building blocks, separated by a semicolon:
// <type> <host>:<port> where
// <type> = "DIRECT" | "PROXY" | "SOCKS" | "HTTP" | "HTTPS" | "SOCKS4"
// <host> = a valid DNS hostname or IP address
// <port> = a valid port number.
//
// See https://developer.mozilla.org/en-US/docs/Web/HTTP/Proxy_servers_and_tunneling/Proxy_Auto-Configuration_PAC_file#return_value_format
type Proxies string

// Mode is a proxy mode.
type Mode int

//go:generate stringer -type=Mode

const (
	DIRECT Mode = iota
	PROXY
	SOCKS
	HTTP
	HTTPS
	SOCKS4
)

var noProxy = Proxy{ //nolint:gochecknoglobals // it's a constant
	Mode: DIRECT,
}

// Proxy specifies proxy to be used as parsed from FindProxyForURL result.
// See ParseResult for details.
type Proxy struct {
	Mode Mode
	Host string
	Port string
}

func (s Proxies) String() string {
	return string(s)
}

func (s Proxies) First() (Proxy, error) {
	if s == "" {
		return Proxy{
			Mode: DIRECT,
		}, nil
	}
	spec, _, _ := strings.Cut(string(s), ";")
	p, err := parseProxy(spec)
	if err != nil {
		return noProxy, fmt.Errorf("invalid proxy string at pos %d %q: %w", 0, spec, err)
	}

	return p, nil
}

func (s Proxies) All() ([]Proxy, error) {
	if s == "" {
		return nil, nil
	}
	spec := strings.Split(string(s), ";")

	var (
		res = make([]Proxy, len(spec))
		err error
	)
	for i, v := range spec {
		res[i], err = parseProxy(v)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy string at pos %d %q: %w", i, v, err)
		}
	}
	return res, nil
}

func parseProxy(s string) (Proxy, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return noProxy, nil
	}
	if s == "DIRECT" {
		return Proxy{Mode: DIRECT}, nil
	}

	mode, hostport, ok := strings.Cut(s, " ")
	if !ok {
		return noProxy, fmt.Errorf("missing host:port")
	}
	host, port, err := net.SplitHostPort(hostport)
	if err != nil {
		return noProxy, fmt.Errorf("split host:port: %w", err)
	}
	return Proxy{
		Mode: parseMode(mode),
		Host: host,
		Port: port,
	}, nil
}

func parseMode(s string) Mode {
	switch s {
	case "DIRECT":
		return DIRECT
	case "PROXY":
		return PROXY
	case "SOCKS":
		return SOCKS
	case "HTTP":
		return HTTP
	case "HTTPS":
		return HTTPS
	case "SOCKS4":
		return SOCKS4
	default:
		return DIRECT
	}
}