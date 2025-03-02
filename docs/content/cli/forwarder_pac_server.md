---
id: server
title: forwarder pac server
weight: 103
---

# Forwarder Pac Server

Usage: `forwarder pac server --pac <file|url> [--protocol <http|https|h2>] [--address <host:port>] [flags]`

Start HTTP server that serves a PAC file.
You can start HTTP, HTTPS or H2 (HTTPS) server.
The server may be protected by basic authentication.
If you start an HTTPS server and you don't provide a certificate,
the server will generate a self-signed certificate on startup.

The PAC file can be specified as a file path or URL with scheme "file", "http" or "https".
The PAC file must contain FindProxyForURL or FindProxyForURLEx and must be valid.
Alerts are ignored.


**Note:** You can also specify the options as YAML, JSON or TOML file using `--config-file` flag.
You can generate a config file by running `forwarder pac server config-file` command.


## Examples

```
  # HTTP server with basic authentication
  forwarder pac server --pac pac.js --basic-auth user:pass

  # HTTPS server with self-signed certificate
  forwarder pac server --pac pac.js --protocol https --address localhost:80443

  # HTTPS server with custom certificate
  forwarder pac server --pac pac.js --protocol https --address localhost:80443 --tls-cert-file cert.pem --tls-key-file key.pem

```

## Server options

### `--address` {#address}

* Environment variable: `FORWARDER_ADDRESS`
* Value Format: `<host:port>`
* Default value: `:8080`

The server address to listen on.
If the host is empty, the server will listen on all available interfaces.

### `--basic-auth` {#basic-auth}

* Environment variable: `FORWARDER_BASIC_AUTH`
* Value Format: `<username[:password]>`

Basic authentication credentials to protect the server.

### `--idle-timeout` {#idle-timeout}

* Environment variable: `FORWARDER_IDLE_TIMEOUT`
* Value Format: `<duration>`
* Default value: `1h0m0s`

The maximum amount of time to wait for the next request before closing connection.

### `--protocol` {#protocol}

* Environment variable: `FORWARDER_PROTOCOL`
* Value Format: `<http|https|h2>`
* Default value: `http`

The server protocol.
For https and h2 protocols, if TLS certificate is not specified, the server will use a self-signed certificate.

### `--read-header-timeout` {#read-header-timeout}

* Environment variable: `FORWARDER_READ_HEADER_TIMEOUT`
* Value Format: `<duration>`
* Default value: `1m0s`

The amount of time allowed to read request headers.

### `--read-limit` {#read-limit}

* Environment variable: `FORWARDER_READ_LIMIT`
* Value Format: `<bandwidth>`
* Default value: `0`

Global read rate limit in bytes per second i.e.
how many bytes per second you can receive from a proxy.
Accepts binary format (e.g.
1.5Ki, 1Mi, 3.6Gi).

### `--shutdown-timeout` {#shutdown-timeout}

* Environment variable: `FORWARDER_SHUTDOWN_TIMEOUT`
* Value Format: `<duration>`
* Default value: `30s`

The maximum amount of time to wait for the server to drain connections before closing.
Zero means no limit.

### `--tls-cert-file` {#tls-cert-file}

* Environment variable: `FORWARDER_TLS_CERT_FILE`
* Value Format: `<path or base64>`

TLS certificate to use if the server protocol is https or h2.

Syntax:

- File: `/path/to/file.pac`
- Embed: `data:base64,<base64 encoded data>`

### `--tls-handshake-timeout` {#tls-handshake-timeout}

* Environment variable: `FORWARDER_TLS_HANDSHAKE_TIMEOUT`
* Value Format: `<duration>`
* Default value: `0s`

The maximum amount of time to wait for a TLS handshake before closing connection.
Zero means no limit.

### `--tls-key-file` {#tls-key-file}

* Environment variable: `FORWARDER_TLS_KEY_FILE`
* Value Format: `<path or base64>`

TLS private key to use if the server protocol is https or h2.

Syntax:

- File: `/path/to/file.pac`
- Embed: `data:base64,<base64 encoded data>`

### `--write-limit` {#write-limit}

* Environment variable: `FORWARDER_WRITE_LIMIT`
* Value Format: `<bandwidth>`
* Default value: `0`

Global write rate limit in bytes per second i.e.
how many bytes per second you can send to proxy.
Accepts binary format (e.g.
1.5Ki, 1Mi, 3.6Gi).

## Proxy options

### `-p, --pac` {#pac}

* Environment variable: `FORWARDER_PAC`
* Value Format: `<path or URL>`
* Default value: `file://pac.js`

Proxy Auto-Configuration file to use for upstream proxy selection.

Syntax:

- File: `/path/to/file.pac`
- URL: `http://example.com/proxy.pac`
- Embed: `data:base64,<base64 encoded data>`
- Stdin: `-`

## DNS options

### `--dns-round-robin` {#dns-round-robin}

* Environment variable: `FORWARDER_DNS_ROUND_ROBIN`
* Value Format: `<value>`
* Default value: `false`

If more than one DNS server is specified with the --dns-server flag, passing this flag will enable round-robin selection.

### `-n, --dns-server` {#dns-server}

* Environment variable: `FORWARDER_DNS_SERVER`
* Value Format: `<ip>[:<port>]`

DNS server(s) to use instead of system default.
There are two execution policies, when more then one server is specified.
Fallback: the first server in a list is used as primary, the rest are used as fallbacks.
Round robin: the servers are used in a round-robin fashion.
The port is optional, if not specified the default port is 53.

### `--dns-timeout` {#dns-timeout}

* Environment variable: `FORWARDER_DNS_TIMEOUT`
* Value Format: `<duration>`
* Default value: `5s`

Timeout for dialing DNS servers.
Only used if DNS servers are specified.

## HTTP client options

### `--cacert-file` {#cacert-file}

* Environment variable: `FORWARDER_CACERT_FILE`
* Value Format: `<path or base64>`

Add your own CA certificates to verify against.
The system root certificates will be used in addition to any certificates in this list.
Use this flag multiple times to specify multiple CA certificate files.

Syntax:

- File: `/path/to/file.pac`
- Embed: `data:base64,<base64 encoded data>`

### `--http-dial-attempts` {#http-dial-attempts}

* Environment variable: `FORWARDER_HTTP_DIAL_ATTEMPTS`
* Value Format: `<int>`
* Default value: `3`

The number of attempts to dial the network address.

### `--http-dial-backoff` {#http-dial-backoff}

* Environment variable: `FORWARDER_HTTP_DIAL_BACKOFF`
* Value Format: `<duration>`
* Default value: `1s`

The amount of time to wait between dial attempts.

### `--http-dial-timeout` {#http-dial-timeout}

* Environment variable: `FORWARDER_HTTP_DIAL_TIMEOUT`
* Value Format: `<duration>`
* Default value: `25s`

The maximum amount of time a dial will wait for a connect to complete.
With or without a timeout, the operating system may impose its own earlier timeout.
For instance, TCP timeouts are often around 3 minutes.

### `--http-idle-conn-timeout` {#http-idle-conn-timeout}

* Environment variable: `FORWARDER_HTTP_IDLE_CONN_TIMEOUT`
* Value Format: `<duration>`
* Default value: `1m30s`

The maximum amount of time an idle (keep-alive) connection will remain idle before closing itself.
Zero means no limit.

### `--http-response-header-timeout` {#http-response-header-timeout}

* Environment variable: `FORWARDER_HTTP_RESPONSE_HEADER_TIMEOUT`
* Value Format: `<duration>`
* Default value: `0s`

The amount of time to wait for a server's response headers after fully writing the request (including its body, if any).This time does not include the time to read the response body.
Zero means no limit.

### `--http-tls-handshake-timeout` {#http-tls-handshake-timeout}

* Environment variable: `FORWARDER_HTTP_TLS_HANDSHAKE_TIMEOUT`
* Value Format: `<duration>`
* Default value: `10s`

The maximum amount of time waiting to wait for a TLS handshake.
Zero means no limit.

### `--http-tls-keylog-file` {#http-tls-keylog-file}

* Environment variable: `FORWARDER_HTTP_TLS_KEYLOG_FILE`
* Value Format: `<path>`

File to log TLS master secrets in NSS key log format.
By default, the value is taken from the SSLKEYLOGFILE environment variable.
It can be used to allow external programs such as Wireshark to decrypt TLS connections.

### `--insecure` {#insecure}

* Environment variable: `FORWARDER_INSECURE`
* Value Format: `<value>`
* Default value: `false`

Don't verify the server's certificate chain and host name.
Enable to work with self-signed certificates.

## Logging options

### `--log-file` {#log-file}

* Environment variable: `FORWARDER_LOG_FILE`
* Value Format: `<path>`

Path to the log file, if empty, logs to stdout.
The file is reopened on SIGHUP to allow log rotation using external tools.

### `--log-http` {#log-http}

* Environment variable: `FORWARDER_LOG_HTTP`
* Value Format: `<none|short-url|url|headers|body|errors>,...`
* Default value: `errors`

HTTP request and response logging mode.

Modes: 

- none: no logging
- short-url: logs [scheme://]host[/path] instead of the full URL
- url: logs the full URL including query parameters
- headers: logs request line and headers
- body: logs request line, headers, and body
- errors: logs request line and headers if status code is greater than or equal to 500

Modes for different modules can be specified separated by commas.
The following example specifies that the API module logs errors, the proxy module logs headers, and anything else logs full URL.

```
--log-http=api:errors,proxy:headers,url
```

### `--log-level` {#log-level}

* Environment variable: `FORWARDER_LOG_LEVEL`
* Value Format: `<error|info|debug>`
* Default value: `info`

Log level.

