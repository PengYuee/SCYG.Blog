package httpserver

import "time"

// ConfigSnapshot is an immutable value copy of the owned net/http configuration.
type ConfigSnapshot struct {
	address           string
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	writeTimeout      time.Duration
	idleTimeout       time.Duration
	maxHeaderBytes    int
}

// Address returns the configured bind address.
func (snapshot ConfigSnapshot) Address() string { return snapshot.address }

// ReadHeaderTimeout returns the configured header deadline.
func (snapshot ConfigSnapshot) ReadHeaderTimeout() time.Duration { return snapshot.readHeaderTimeout }

// ReadTimeout returns the configured request read deadline.
func (snapshot ConfigSnapshot) ReadTimeout() time.Duration { return snapshot.readTimeout }

// WriteTimeout returns the configured response write deadline.
func (snapshot ConfigSnapshot) WriteTimeout() time.Duration { return snapshot.writeTimeout }

// IdleTimeout returns the configured keep-alive deadline.
func (snapshot ConfigSnapshot) IdleTimeout() time.Duration { return snapshot.idleTimeout }

// MaxHeaderBytes returns the configured request-header byte limit.
func (snapshot ConfigSnapshot) MaxHeaderBytes() int { return snapshot.maxHeaderBytes }
