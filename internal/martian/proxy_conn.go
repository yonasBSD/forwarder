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
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/saucelabs/forwarder/internal/martian/log"
	"github.com/saucelabs/forwarder/internal/martian/proxyutil"
	"github.com/saucelabs/forwarder/utils/maps"
)

type proxyConn struct {
	*Proxy
	brw    *bufio.ReadWriter
	conn   net.Conn
	secure bool
	cs     tls.ConnectionState
}

func newProxyConn(p *Proxy, conn net.Conn) *proxyConn {
	return &proxyConn{
		Proxy: p,
		brw:   bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
		conn:  conn,
	}
}

func (p *proxyConn) maybeHandshakeTLS() error {
	tconn, ok := p.conn.(*tls.Conn)
	if !ok {
		return nil
	}

	ctx := context.Background()
	if p.TLSHandshakeTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), p.TLSHandshakeTimeout)
		defer cancel()
	}
	if err := tconn.HandshakeContext(ctx); err != nil {
		return err
	}

	p.secure = true
	p.cs = tconn.ConnectionState()

	return nil
}

func (p *proxyConn) readRequest() (*http.Request, error) {
	var idleDeadline time.Time // or zero if none
	if d := p.idleTimeout(); d > 0 {
		idleDeadline = time.Now().Add(d)
	}
	if deadlineErr := p.conn.SetReadDeadline(idleDeadline); deadlineErr != nil {
		log.Error(context.TODO(), "can't set idle deadline", "error", deadlineErr)
	}

	// Wait for the connection to become readable before trying to
	// read the next request. This prevents a ReadHeaderTimeout or
	// ReadTimeout from starting until the first bytes of the next request
	// have been received.
	if _, err := p.brw.Peek(1); err != nil {
		return nil, err
	}

	var (
		wholeReqDeadline time.Time // or zero if none
		hdrDeadline      time.Time // or zero if none
	)
	t0 := time.Now()
	if d := p.readHeaderTimeout(); d > 0 {
		hdrDeadline = t0.Add(d)
	}
	if d := p.ReadTimeout; d > 0 {
		wholeReqDeadline = t0.Add(d)
	}

	if deadlineErr := p.conn.SetReadDeadline(hdrDeadline); deadlineErr != nil {
		log.Error(context.TODO(), "can't set read header deadline", "error", deadlineErr)
	}

	req, err := http.ReadRequest(p.brw.Reader)
	if err != nil {
		return nil, err
	}

	fixConnectReqContentLength(req)
	if p.secure {
		req.TLS = &p.cs
	}
	req.RemoteAddr = p.conn.RemoteAddr().String()
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}

	req = req.WithContext(withTraceID(p.BaseContext, newTraceID(req.Header.Get(p.RequestIDHeader))))

	// Adjust the read deadline if necessary.
	if !hdrDeadline.Equal(wholeReqDeadline) {
		if deadlineErr := p.conn.SetReadDeadline(wholeReqDeadline); deadlineErr != nil {
			log.Error(context.TODO(), "can't set read deadline", "error", deadlineErr)
		}
	}

	return req, nil
}

func (p *proxyConn) handleMITM(req *http.Request) error {
	ctx := req.Context()

	log.Debug(ctx, "mitm: attempting MITM")

	res := newConnectResponse(req)

	if err := p.modifyResponse(res); err != nil {
		log.Debug(ctx, "error modifying CONNECT response", "error", err)
		return p.writeResponse(p.errorResponse(req, err))
	}

	if err := p.writeResponse(res); err != nil {
		return err
	}
	// Successful CONNECT response does not invoke trace.
	p.traceWroteResponse(res, nil)

	b, err := p.brw.Peek(1)
	if err != nil {
		if isClosedConnError(err) {
			log.Debug(ctx, "mitm: connection closed prematurely", "error", err)
		} else {
			log.Error(ctx, "mitm: failed to peek connection", "host", req.Host, "error", err)
		}
		return errClose
	}

	// Drain the rest of the buffered data.
	buf := make([]byte, p.brw.Reader.Buffered())
	if _, err := p.brw.Read(buf); err != nil {
		log.Error(ctx, "mitm: failed to drain buffer", "error", err)
		return errClose
	}

	// 22 is the TLS handshake.
	// https://tools.ietf.org/html/rfc5246#section-6.2.1
	if len(b) > 0 && b[0] == 22 {
		// Prepend the previously read data to be read again by http.ReadRequest.
		tlsconn := tls.Server(&peekedConn{
			p.conn,
			io.MultiReader(bytes.NewReader(buf), p.conn),
		}, p.MITMConfig.TLSForHost(req.Context(), req.Host))

		var hctx context.Context
		if p.MITMTLSHandshakeTimeout > 0 {
			var hcancel context.CancelFunc
			hctx, hcancel = context.WithTimeout(ctx, p.MITMTLSHandshakeTimeout)
			defer hcancel()
		} else {
			hctx = ctx
		}
		if err = tlsconn.HandshakeContext(hctx); err != nil {
			p.MITMConfig.HandshakeErrorCallback(req, err)
			if isClosedConnError(err) {
				log.Debug(ctx, "mitm: connection closed prematurely", "error", err)
			} else {
				log.Error(ctx, "mitm: failed to handshake connection", "host", req.Host, "error", err)
			}
			return errClose
		}

		cs := tlsconn.ConnectionState()
		log.Debug(ctx, "mitm: negotiated protocol", "protocol", cs.NegotiatedProtocol)

		if cs.NegotiatedProtocol == "h2" {
			return p.MITMConfig.H2Config().Proxy(p.closeCh, tlsconn, req.URL)
		}

		p.brw.Writer.Reset(tlsconn)
		p.brw.Reader.Reset(tlsconn)

		p.conn = tlsconn
		p.secure = true
		p.cs = cs

		return nil
	}

	// Prepend the previously read data to be read again by http.ReadRequest.
	p.brw.Reader.Reset(io.MultiReader(bytes.NewReader(buf), p.conn))
	return nil
}

func (p *proxyConn) handleConnectRequest(req *http.Request) error {
	ctx := req.Context()
	log.Debug(ctx, "read CONNECT request", "host", req.URL.Host)

	terminateTLS := shouldTerminateTLS(req)
	req.Header.Del(terminateTLSHeader)

	if err := p.modifyRequest(req); err != nil {
		log.Debug(ctx, "error modifying CONNECT request", "error", err)
		return p.writeErrorResponse(req, err)
	}

	if p.shouldMITM(req) {
		return p.handleMITM(req)
	}

	log.Debug(ctx, "attempting to establish CONNECT tunnel", "host", req.URL.Host)
	res, crw, cerr := p.Connect(ctx, req, terminateTLS)
	if res != nil {
		defer res.Body.Close()
	}
	if crw != nil {
		defer crw.Close()
	}
	if cerr != nil {
		log.Error(ctx, "failed to CONNECT", "error", cerr)
		return p.writeErrorResponse(req, cerr)
	}

	if err := p.modifyResponse(res); err != nil {
		log.Debug(ctx, "error modifying CONNECT response", "error", err)
		return p.writeErrorResponse(req, err)
	}

	if res.StatusCode/100 != 2 {
		log.Info(ctx, "CONNECT rejected", "status code", res.StatusCode)
		return p.writeResponse(res)
	}

	if err := p.tunnel("CONNECT", res, crw); err != nil {
		log.Error(ctx, "CONNECT tunnel", "error", err)
	}

	return errClose
}

func (p *proxyConn) handleUpgradeResponse(res *http.Response) error {
	resUpType := upgradeType(res.Header)

	uconn, ok := res.Body.(io.ReadWriteCloser)
	if !ok {
		log.Error(res.Request.Context(), "internal error: switching protocols response with non-writable body")
		p.traceWroteResponse(res, errors.New("switching protocols response with non-writable body"))
		return errClose
	}
	res.Body = panicBody

	if err := p.tunnel(resUpType, res, uconn); err != nil {
		log.Error(res.Request.Context(), "upgrade tunnel error", "type", resUpType, "error", err)
	}

	return errClose
}

func (p *proxyConn) tunnel(name string, res *http.Response, crw io.ReadWriteCloser) error {
	if err := p.writeResponse(res); err != nil {
		return err
	}
	if err := drainBuffer(crw, p.brw.Reader); err != nil {
		err := fmt.Errorf("got error while draining read buffer: %w", err)
		p.traceWroteResponse(res, err)
		return err
	}

	ctx := res.Request.Context()

	log.Debug(ctx, "switched protocols, proxying traffic", "name", name)
	bicopy(ctx,
		copier{"upstream " + name, crw, p.conn},
		copier{"downstream " + name, p.conn, crw},
	)
	log.Debug(ctx, "closed tunnel", "name", name, "duration", ContextDuration(ctx))

	p.traceWroteResponse(res, nil)

	return nil
}

func (p *proxyConn) handle() error {
	req, err := p.readRequest()
	p.traceReadRequest(req, err)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errClose
		}

		if isClosedConnError(err) {
			log.Debug(context.TODO(), "connection closed prematurely while reading request", "error", err)
		} else {
			log.Error(context.TODO(), "got error while reading request", "error", err)
		}
		return errClose
	}
	defer req.Body.Close()

	if p.closing() {
		return errClose
	}

	if req.Method == http.MethodConnect {
		return p.handleConnectRequest(req)
	}

	ctx := req.Context()

	p.fixRequestScheme(req)

	reqUpType := upgradeType(req.Header)
	if reqUpType != "" {
		log.Debug(ctx, "upgrade request", "type", reqUpType)
	}

	if err := p.modifyRequest(req); err != nil {
		log.Debug(ctx, "error modifying request", "error", err)
		return p.writeErrorResponse(req, err)
	}

	// after stripping all the hop-by-hop connection headers above, add back any
	// necessary for protocol upgrades, such as for websockets.
	if reqUpType != "" {
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", reqUpType)
	}

	// perform the HTTP roundtrip
	res, err := p.roundTrip(req)
	if err != nil {
		if isClosedConnError(err) {
			log.Debug(ctx, "connection closed prematurely", "error", err)
		} else {
			log.Error(ctx, "failed to round trip", "host", req.Host, "method", req.Method, "path", req.URL.Path, "error", err)
		}
		return p.writeErrorResponse(req, err)
	}

	defer res.Body.Close()

	// set request to original request manually, res.Request may be changed in transport.
	// see https://github.com/google/martian/issues/298
	res.Request = req

	resUpType := upgradeType(res.Header)
	if resUpType != "" {
		log.Debug(ctx, "upgrade response", "type", resUpType)
	}

	if err := p.modifyResponse(res); err != nil {
		log.Debug(ctx, "error modifying response", "error", err)
		return p.writeErrorResponse(req, err)
	}

	// after stripping all the hop-by-hop connection headers above, add back any
	// necessary for protocol upgrades, such as for websockets.
	if resUpType != "" {
		res.Header.Set("Connection", "Upgrade")
		res.Header.Set("Upgrade", resUpType)
	}

	// deal with 101 Switching Protocols responses: (WebSocket, h2c, etc)
	if res.StatusCode == http.StatusSwitchingProtocols {
		return p.handleUpgradeResponse(res)
	}

	return p.writeResponse(res)
}

func (p *proxyConn) writeErrorResponse(req *http.Request, err error) error {
	res := maybeConnectErrorResponse(err)
	if res == nil {
		res = p.errorResponse(req, err)
	}
	if err := p.modifyResponse(res); err != nil {
		log.Error(req.Context(), "error modifying error response", "error", err)
		if !p.WithoutWarning {
			proxyutil.Warning(res.Header, err)
		}
	}
	return p.writeResponse(res)
}

func (p *proxyConn) writeResponse(res *http.Response) error {
	req := res.Request
	ctx := req.Context()

	if p.WriteTimeout > 0 {
		if deadlineErr := p.conn.SetWriteDeadline(time.Now().Add(p.WriteTimeout)); deadlineErr != nil {
			log.Error(ctx, "can't set write deadline", "error", deadlineErr)
		}
		defer func() {
			if deadlineErr := p.conn.SetWriteDeadline(time.Time{}); deadlineErr != nil {
				log.Error(ctx, "can't clear write deadline", "error", deadlineErr)
			}
		}()
	}

	if p.closing() {
		res.Close = true
	} else {
		if req.Close {
			res.Close = true
		}
		// Support CONNECT over HTTP/1.0.
		// If connect is successful, the connection should not be closed.
		if req.Method == http.MethodConnect && res.StatusCode/100 == 2 {
			res.Close = false
		}
	}

	if res.Close {
		res.Header.Add("Connection", "close")
	}

	var err error
	switch {
	case req.Method == http.MethodConnect && res.StatusCode/100 == 2:
		err = writeConnectOKResponse(p.brw.Writer)
	case isHeaderOnlySpec(res):
		// The http package is misbehaving when writing a HEAD response.
		// See https://github.com/golang/go/issues/62015 for details.
		// This works around the issue by writing the response manually.
		err = writeHeaderOnlyResponse(p.brw.Writer, res)
	default:
		// Add support for Server Sent Events - relay HTTP chunks and flush after each chunk.
		// This is safe for events that are smaller than the buffer io.Copy uses (32KB).
		// If the event is larger than the buffer, the event will be split into multiple chunks.
		switch {
		case isTextEventStream(res):
			w := newPatternFlushWriter(p.brw.Writer, p.brw.Writer, sseFlushPattern)
			err = res.Write(w)
		case shouldChunk(res):
			w := newPatternFlushWriter(p.brw.Writer, p.brw.Writer, chunkFlushPattern)
			err = res.Write(w)
		default:
			err = res.Write(p.brw)
		}
	}
	if err != nil {
		p.brw.Flush() // flush any remaining data
	} else {
		err = p.brw.Flush()
	}

	// traceWroteResponse must not be called for:
	//	- a successful CONNECT request
	//	- a successful protocol upgrade (101 Switching Protocols)
	// In these cases, only the headers are written here; the rest of the data flows
	// through a raw TCP tunnel. Therefore, traceWroteResponse should be invoked
	// only after the tunnel (and thus the response body) has been fully closed.
	if !skipTraceWroteResponse(res, err) {
		p.traceWroteResponse(res, err)
	}

	if err != nil {
		if isClosedConnError(err) {
			log.Debug(ctx, "connection closed prematurely while writing response", "error", err)
		} else {
			log.Error(ctx, "got error while writing response", "error", err)
		}
		return errClose
	}

	if res.Close {
		log.Debug(ctx, "closing connection")
		return errClose
	}

	return nil
}

func skipTraceWroteResponse(res *http.Response, err error) bool {
	// Do not skip traceWroteResponse on error.
	if err != nil {
		return false
	}

	// Skip traceWroteResponse on successful CONNECT.
	req := res.Request
	if req.Method == http.MethodConnect && res.StatusCode/100 == 2 {
		return true
	}

	// Skip traceeWroteResponse on successful protocol upgrade.
	if res.StatusCode == http.StatusSwitchingProtocols {
		return true
	}

	return false
}

// writeHeaderOnlyResponse writes the status line and header of r to w.
func writeHeaderOnlyResponse(w io.Writer, res *http.Response) error {
	// Status line
	text := res.Status
	if text == "" {
		text = http.StatusText(res.StatusCode)
		if text == "" {
			text = "status code " + strconv.Itoa(res.StatusCode)
		}
	} else {
		// Just to reduce stutter, if user set res.Status to "200 OK" and StatusCode to 200.
		// Not important.
		text = strings.TrimPrefix(text, strconv.Itoa(res.StatusCode)+" ")
	}

	if _, err := fmt.Fprintf(w, "HTTP/%d.%d %03d %s\r\n", res.ProtoMajor, res.ProtoMinor, res.StatusCode, text); err != nil {
		return err
	}

	// Header
	if err := res.Header.Write(w); err != nil {
		return err
	}

	// Add Trailer header if needed
	if len(res.Trailer) > 0 {
		if _, err := io.WriteString(w, "Trailer: "); err != nil {
			return err
		}

		for i, k := range maps.Keys(res.Trailer) {
			if i > 0 {
				if _, err := io.WriteString(w, ", "); err != nil {
					return err
				}
			}
			if _, err := io.WriteString(w, k); err != nil {
				return err
			}
		}
	}

	// End-of-header
	if _, err := io.WriteString(w, "\r\n"); err != nil {
		return err
	}

	return nil
}
