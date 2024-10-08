// Copyright 2022-2024 Sauce Labs Inc., all rights reserved.
//
// Copyright 2015 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package martian

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/saucelabs/forwarder/dialvia"
	"github.com/saucelabs/forwarder/internal/martian/log"
)

func fixConnectReqContentLength(req *http.Request) {
	if req.Method != http.MethodConnect {
		return
	}

	if req.Header.Get("Content-Length") != "" {
		log.Infof(req.Context(), "CONNECT request with Content-Length: %s, ignoring content length",
			req.Header.Get("Content-Length"))
	}

	req.ContentLength = -1
}

// ErrConnectFallback is returned by a ConnectFunc to indicate
// that the CONNECT request should be handled by martian.
var ErrConnectFallback = errors.New("martian: connect fallback")

// ConnectFunc dials a network connection for a CONNECT request.
// If the returned net.Conn is not nil, the response must be not nil.
type ConnectFunc func(req *http.Request) (*http.Response, io.ReadWriteCloser, error)

func (p *Proxy) Connect(ctx context.Context, req *http.Request, terminateTLS bool) (res *http.Response, crw io.ReadWriteCloser, cerr error) {
	if p.ConnectFunc != nil {
		res, crw, cerr = p.ConnectFunc(req)
	}
	if p.ConnectFunc == nil || errors.Is(cerr, ErrConnectFallback) {
		var cconn net.Conn
		res, cconn, cerr = p.connect(req)

		if cconn != nil {
			crw = cconn

			if terminateTLS {
				log.Debugf(ctx, "attempting to terminate TLS on CONNECT tunnel: %s", req.URL.Host)
				tconn := tls.Client(cconn, p.clientTLSConfig())
				if err := tconn.Handshake(); err == nil {
					crw = tconn
				} else {
					log.Errorf(ctx, "failed to terminate TLS on CONNECT tunnel: %v", err)
					cerr = err
				}
			}
		}
	}

	return
}

func (p *Proxy) connect(req *http.Request) (*http.Response, net.Conn, error) {
	ctx := req.Context()

	var proxyURL *url.URL
	if p.ProxyURL != nil {
		u, err := p.ProxyURL(req)
		if err != nil {
			return nil, nil, err
		}
		proxyURL = u
	}

	if proxyURL == nil {
		log.Debugf(ctx, "CONNECT to host directly: %s", req.URL.Host)

		conn, err := p.DialContext(ctx, "tcp", req.URL.Host)
		if err != nil {
			return nil, nil, err
		}

		return newConnectResponse(req), conn, nil
	}

	switch proxyURL.Scheme {
	case "http", "https":
		return p.connectHTTP(req, proxyURL)
	case "socks5":
		return p.connectSOCKS5(req, proxyURL)
	default:
		return nil, nil, fmt.Errorf("unsupported proxy scheme: %s", proxyURL.Scheme)
	}
}

func (p *Proxy) connectHTTP(req *http.Request, proxyURL *url.URL) (res *http.Response, conn net.Conn, err error) {
	ctx := req.Context()

	log.Debugf(ctx, "CONNECT with upstream HTTP proxy: %s", proxyURL.Host)

	var d *dialvia.HTTPProxyDialer
	if proxyURL.Scheme == "https" {
		d = dialvia.HTTPSProxy(p.DialContext, proxyURL, p.clientTLSConfig())
	} else {
		d = dialvia.HTTPProxy(p.DialContext, proxyURL)
	}
	d.Timeout = p.ConnectTimeout
	d.ProxyConnectHeader = req.Header.Clone()

	res, conn, err = d.DialContextR(ctx, "tcp", req.URL.Host)

	if res != nil {
		if res.StatusCode/100 == 2 {
			res.Body.Close()
			return newConnectResponse(req), conn, nil
		}

		// If the proxy returns a non-2xx response, return it to the client.
		// But first, replace the Request with the original request.
		res.Request = req
	}

	return res, conn, err
}

func (p *Proxy) clientTLSConfig() *tls.Config {
	if tr, ok := p.rt.(*http.Transport); ok && tr.TLSClientConfig != nil {
		return tr.TLSClientConfig.Clone()
	}

	return &tls.Config{}
}

func (p *Proxy) connectSOCKS5(req *http.Request, proxyURL *url.URL) (*http.Response, net.Conn, error) {
	ctx := req.Context()

	log.Debugf(ctx, "CONNECT with upstream SOCKS5 proxy: %s", proxyURL.Host)

	d := dialvia.SOCKS5Proxy(p.DialContext, proxyURL)
	d.Timeout = p.ConnectTimeout

	conn, err := d.DialContext(ctx, "tcp", req.URL.Host)
	if err != nil {
		return nil, nil, err
	}

	return newConnectResponse(req), conn, nil
}

func newConnectResponse(req *http.Request) *http.Response {
	ok := http.StatusOK
	return &http.Response{
		Status:     fmt.Sprintf("%d %s", ok, http.StatusText(ok)),
		StatusCode: ok,
		Proto:      req.Proto,
		ProtoMajor: req.ProtoMajor,
		ProtoMinor: req.ProtoMinor,

		Header: make(http.Header),

		Body:          http.NoBody,
		ContentLength: -1,

		Request: req,
	}
}

const terminateTLSHeader = "X-Martian-Terminate-Tls"

func shouldTerminateTLS(req *http.Request) bool {
	h := req.Header.Get(terminateTLSHeader)
	if h == "" {
		return false
	}
	b, _ := strconv.ParseBool(h)
	return b
}
