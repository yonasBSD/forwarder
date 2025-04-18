---
id: eval
title: forwarder pac eval
weight: 102
---

# Forwarder Pac Eval

Usage: `forwarder pac eval --pac <file|url> [flags] <url>...`

Evaluate a PAC file for given URL (or URLs).
The output is a list of proxy strings, one per URL.
The PAC file can be specified as a file path or URL with scheme "file", "http" or "https".
The PAC file must contain FindProxyForURL or FindProxyForURLEx and must be valid.
Alerts are written to stderr.


**Note:** You can also specify the options as YAML, JSON or TOML file using `--config-file` flag.
You can generate a config file by running `forwarder pac eval config-file` command.


## Examples

```
  # Evaluate PAC file for multiple URLs
  forwarder pac eval --pac pac.js https://www.google.com https://www.facebook.com

```

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

