package config

import (
	"net"
	"strconv"
	"time"
)

// Agent is the immutable internal-network integration configuration.
type Agent struct {
	enabled                                           bool
	target, blogContentListen                         string
	unaryTimeout, sseIdleTimeout, grpcShutdownTimeout time.Duration
}

// Enabled determines whether clients, routes, and listeners are constructed.
func (a Agent) Enabled() bool { return a.enabled }

// Target returns the AgentControl dial address.
func (a Agent) Target() string { return a.target }

// BlogContentListen returns the internal BlogContent bind address.
func (a Agent) BlogContentListen() string { return a.blogContentListen }

// UnaryTimeout bounds unary calls and subscription establishment.
func (a Agent) UnaryTimeout() time.Duration { return a.unaryTimeout }

// SSEIdleTimeout bounds receive inactivity and each frame write.
func (a Agent) SSEIdleTimeout() time.Duration { return a.sseIdleTimeout }

// GRPCShutdownTimeout gives gRPC an independent drain budget.
func (a Agent) GRPCShutdownTimeout() time.Duration { return a.grpcShutdownTimeout }

// Agent returns the integration settings.
func (c Config) Agent() Agent { return c.agent }

type rawAgent struct {
	Enabled             bool          `mapstructure:"enabled"`
	Target              string        `mapstructure:"target"`
	BlogContentListen   string        `mapstructure:"blog_content_listen"`
	UnaryTimeout        time.Duration `mapstructure:"unary_timeout"`
	SSEIdleTimeout      time.Duration `mapstructure:"sse_idle_timeout"`
	GRPCShutdownTimeout time.Duration `mapstructure:"grpc_shutdown_timeout"`
}

func validateAgent(raw rawAgent) (Agent, error) {
	if raw.Enabled {
		for _, item := range []struct{ key, value string }{{"agent.target", raw.Target}, {"agent.blog_content_listen", raw.BlogContentListen}} {
			host, port, err := net.SplitHostPort(item.value)
			number, parseErr := strconv.Atoi(port)
			if err != nil || parseErr != nil || host == "" || number < 1 || number > 65535 {
				return Agent{}, invalid(item.key, "需要有效的 host:port")
			}
		}
		for _, item := range []struct {
			key   string
			value time.Duration
		}{{"agent.unary_timeout", raw.UnaryTimeout}, {"agent.sse_idle_timeout", raw.SSEIdleTimeout}, {"agent.grpc_shutdown_timeout", raw.GRPCShutdownTimeout}} {
			if item.value <= 0 {
				return Agent{}, invalid(item.key, "必须大于零")
			}
		}
	}
	return Agent{raw.Enabled, raw.Target, raw.BlogContentListen, raw.UnaryTimeout, raw.SSEIdleTimeout, raw.GRPCShutdownTimeout}, nil
}
