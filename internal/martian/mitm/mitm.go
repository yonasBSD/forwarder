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

// Package mitm provides tooling for MITMing TLS connections. It provides
// tooling to create CA certs and generate TLS configs that can be used to MITM
// a TLS connection with a provided CA certificate.
package mitm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"time"

	"github.com/saucelabs/forwarder/internal/martian/h2"
	"github.com/saucelabs/forwarder/internal/martian/log"
)

// MaxSerialNumber is the upper boundary that is used to create unique serial
// numbers for the certificate. This can be any unsigned integer up to 20
// bytes (2^(8*20)-1).
var MaxSerialNumber = big.NewInt(0).SetBytes(bytes.Repeat([]byte{255}, 20))

// Config is a set of configuration values that are used to build TLS configs
// capable of MITM.
type Config struct {
	ca                     *x509.Certificate
	capriv                 any
	priv                   *rsa.PrivateKey
	keyID                  []byte
	validity               time.Duration
	org                    string
	h2Config               *h2.Config
	certs                  Cache
	roots                  *x509.CertPool
	handshakeErrorCallback func(*http.Request, error)
}

// NewAuthority creates a new CA certificate and associated
// private key.
func NewAuthority(name, organization string, validity time.Duration) (*x509.Certificate, *rsa.PrivateKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	pub := priv.Public()

	// Subject Key Identifier support for end entity certificate.
	// https://www.ietf.org/rfc/rfc3280.txt (section 4.2.1.2)
	pkixpub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, err
	}
	h := sha256.New()
	h.Write(pkixpub)
	keyID := h.Sum(nil)

	// TODO: keep a map of used serial numbers to avoid potentially reusing a
	// serial multiple times.
	serial, err := rand.Int(rand.Reader, MaxSerialNumber)
	if err != nil {
		return nil, nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   name,
			Organization: []string{organization},
		},
		SubjectKeyId:          keyID,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-validity),
		NotAfter:              time.Now().Add(validity),
		DNSNames:              []string{name},
		IsCA:                  true,
	}

	raw, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		return nil, nil, err
	}

	// Parse certificate bytes so that we have a leaf certificate.
	x509c, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, nil, err
	}

	return x509c, priv, nil
}

// NewConfig creates a MITM config using the CA certificate and
// private key to generate on-the-fly certificates.
func NewConfig(ca *x509.Certificate, privateKey any) (*Config, error) {
	certs, err := NewCache(DefaultCacheConfig())
	if err != nil {
		return nil, err
	}
	return NewConfigWithCache(ca, privateKey, certs)
}

func NewConfigWithCache(ca *x509.Certificate, privateKey any, certs Cache) (*Config, error) {
	roots := x509.NewCertPool()
	roots.AddCert(ca)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	pub := priv.Public()

	// Subject Key Identifier support for end entity certificate.
	// https://www.ietf.org/rfc/rfc3280.txt (section 4.2.1.2)
	pkixpub, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write(pkixpub)
	keyID := h.Sum(nil)

	return &Config{
		ca:       ca,
		capriv:   privateKey,
		priv:     priv,
		keyID:    keyID,
		validity: time.Hour,
		org:      "Martian Proxy",
		certs:    certs,
		roots:    roots,
	}, nil
}

// SetValidity sets the validity window around the current time that the
// certificate is valid for.
func (c *Config) SetValidity(validity time.Duration) {
	c.validity = validity
}

// SetOrganization sets the organization of the certificate.
func (c *Config) SetOrganization(org string) {
	c.org = org
}

// SetH2Config configures processing of HTTP/2 streams.
func (c *Config) SetH2Config(h2Config *h2.Config) {
	c.h2Config = h2Config
}

// H2Config returns the current HTTP/2 configuration.
func (c *Config) H2Config() *h2.Config {
	return c.h2Config
}

// SetHandshakeErrorCallback sets the handshakeErrorCallback function.
func (c *Config) SetHandshakeErrorCallback(cb func(*http.Request, error)) {
	c.handshakeErrorCallback = cb
}

// HandshakeErrorCallback calls the handshakeErrorCallback function in this
// Config, if it is non-nil. Request is the connect request that this handshake
// is being executed through.
func (c *Config) HandshakeErrorCallback(r *http.Request, err error) {
	if c.handshakeErrorCallback != nil {
		c.handshakeErrorCallback(r, err)
	}
}

// CACert returns the CA certificate used to sign the on-the-fly certificates.
func (c *Config) CACert() *x509.Certificate {
	return c.ca
}

// TLS returns a *tls.Config that will generate certificates on-the-fly using
// the SNI extension in the TLS ClientHello.
func (c *Config) TLS(ctx context.Context) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if clientHello.ServerName == "" {
				return nil, errors.New("mitm: SNI not provided, failed to build certificate")
			}

			return c.cert(ctx, clientHello.ServerName)
		},
		NextProtos: []string{"http/1.1"},
	}
}

// TLSForHost returns a *tls.Config that will generate certificates on-the-fly
// using SNI from the connection, or fall back to the provided hostname.
func (c *Config) TLSForHost(ctx context.Context, hostname string) *tls.Config {
	nextProtos := []string{"http/1.1"}
	if c.h2AllowedHost(hostname) {
		nextProtos = []string{"h2", "http/1.1"}
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			host := clientHello.ServerName
			if host == "" {
				host = hostname
			}

			return c.cert(ctx, host)
		},
		NextProtos: nextProtos,
	}
}

func (c *Config) h2AllowedHost(host string) bool {
	return c.h2Config != nil &&
		c.h2Config.AllowedHostsFilter != nil &&
		c.h2Config.AllowedHostsFilter(host)
}

func (c *Config) cert(ctx context.Context, hostname string) (*tls.Certificate, error) {
	// Remove the port if it exists.
	host, _, err := net.SplitHostPort(hostname)
	if err == nil {
		hostname = host
	}

	tlsc, ok := c.certs.Get(hostname)
	if ok {
		log.Debug(ctx, "mitm: cache hit", "hostname", hostname)

		// Check validity of the certificate for hostname match, expiry, etc. In
		// particular, if the cached certificate has expired, create a new one.
		if _, err := tlsc.Leaf.Verify(x509.VerifyOptions{
			DNSName: hostname,
			Roots:   c.roots,
		}); err == nil {
			return tlsc, nil
		}

		log.Debug(ctx, "mitm: invalid certificate in cache", "hostname", hostname)
	}

	log.Debug(ctx, "mitm: cache miss", "hostname", hostname)

	serial, err := rand.Int(rand.Reader, MaxSerialNumber)
	if err != nil {
		return nil, err
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   hostname,
			Organization: []string{c.org},
		},
		SubjectKeyId:          c.keyID,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		NotBefore:             time.Now().Add(-c.validity),
		NotAfter:              time.Now().Add(c.validity),
	}

	if ip := net.ParseIP(hostname); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{hostname}
	}

	raw, err := x509.CreateCertificate(rand.Reader, tmpl, c.ca, c.priv.Public(), c.capriv)
	if err != nil {
		return nil, err
	}

	// Parse certificate bytes so that we have a leaf certificate.
	x509c, err := x509.ParseCertificate(raw)
	if err != nil {
		return nil, err
	}

	tlsc = &tls.Certificate{
		Certificate: [][]byte{raw, c.ca.Raw},
		PrivateKey:  c.priv,
		Leaf:        x509c,
	}

	c.certs.Add(hostname, tlsc)

	return tlsc, nil
}

// CacheMetrics return the metrics for the certificate cache.
func (c *Config) CacheMetrics() CacheMetrics {
	return CacheMetrics(c.certs.Metrics())
}
