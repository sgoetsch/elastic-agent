// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package remote

import (
	"fmt"
	"time"

	"github.com/elastic/elastic-agent-libs/transport/httpcommon"
)

// Config is the configuration for the client.
type Config struct {
	Protocol Protocol          `config:"protocol" yaml:"protocol,omitempty"`
	SpaceID  string            `config:"space.id" yaml:"space.id,omitempty"`
	Path     string            `config:"path" yaml:"path,omitempty"`
	Host     string            `config:"host" yaml:"host,omitempty"`
	Hosts    []string          `config:"hosts" yaml:"hosts,omitempty"`
	Headers  map[string]string `config:"headers" yaml:"headers,omitempty"`

	Transport httpcommon.HTTPTransportSettings `config:",inline" yaml:",inline"`
}

// Protocol define the protocol to use to make the connection. (Either HTTPS or HTTP)
type Protocol string

const (
	// ProtocolHTTP is HTTP protocol connection.
	ProtocolHTTP Protocol = "http"
	// ProtocolHTTPS is HTTPS protocol connection.
	ProtocolHTTPS Protocol = "https"
)

// Unpack the protocol.
func (p *Protocol) Unpack(from string) error {
	if from != "" && Protocol(from) != ProtocolHTTPS && Protocol(from) != ProtocolHTTP {
		return fmt.Errorf("invalid protocol %q, accepted values are 'http' and 'https'", from)
	}

	*p = Protocol(from)
	return nil
}

// DefaultClientConfig creates default configuration for client.
func DefaultClientConfig() Config {
	transport := httpcommon.DefaultHTTPTransportSettings()
	// Default timeout 10 minutes, expecting Fleet Server to control the long poll with default timeout of 5 minutes
	transport.Timeout = 10 * time.Minute

	return Config{
		Protocol:  ProtocolHTTP,
		Host:      "localhost:5601",
		Path:      "",
		SpaceID:   "",
		Transport: transport,
	}
}

// GetHosts returns the hosts to connect.
//
// This looks first at `Hosts` and then at `Host` when `Hosts` is not defined.
func (c *Config) GetHosts() []string {
	if len(c.Hosts) > 0 {
		return c.Hosts
	}
	return []string{c.Host}
}

// Validate returns an error if the configuration is invalid; nil, otherwise.
func (c *Config) Validate() error {
	if c.Transport.TLS != nil {
		return c.Transport.TLS.Validate()
	}

	return nil
}
