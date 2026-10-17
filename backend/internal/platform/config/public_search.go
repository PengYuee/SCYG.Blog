package config

import (
	"net/url"
	"strings"
	"time"
)

// PublicSearch contains validated public-search credentials and distributed-control settings.
type PublicSearch struct{ raw rawPublicSearch }

// RedisURL is consumed only by the Redis adapter; do not log it.
func (p PublicSearch) RedisURL() string { return p.raw.RedisURL }

// Prefix separates environments sharing Redis.
func (p PublicSearch) Prefix() string { return p.raw.Prefix }

// Timeout bounds each Redis operation without retries.
func (p PublicSearch) Timeout() time.Duration { return p.raw.Timeout }

// TokenTTL bounds anonymous credentials.
func (p PublicSearch) TokenTTL() time.Duration { return p.raw.TokenTTL }

// QueryMaxChars is the public form and service input limit.
func (p PublicSearch) QueryMaxChars() int { return p.raw.QueryMaxChars }

// MaxResults is the public result limit.
func (p PublicSearch) MaxResults() int { return p.raw.MaxResults }

// SearchRate returns the per-minute rate and explicit burst.
func (p PublicSearch) SearchRate() (int, int) { return p.raw.SearchRate, p.raw.SearchBurst }

// SessionRate returns the independent issuance rate and burst.
func (p PublicSearch) SessionRate() (int, int) { return p.raw.SessionRate, p.raw.SessionBurst }

// ReadRate returns the lightweight Run operation rate and burst.
func (p PublicSearch) ReadRate() (int, int) { return p.raw.ReadRate, p.raw.ReadBurst }

// SSEConnections bounds distributed connections for one anonymous Run.
func (p PublicSearch) SSEConnections() int { return p.raw.SSEConnections }

// LeaseTTL returns the connection ownership duration.
func (p PublicSearch) LeaseTTL() time.Duration { return p.raw.LeaseTTL }

// LeaseRenewInterval returns the renewal cadence.
func (p PublicSearch) LeaseRenewInterval() time.Duration { return p.raw.LeaseRenewInterval }

// PublicSearch returns the immutable public-search settings.
func (c Config) PublicSearch() PublicSearch { return c.publicSearch }

// String prevents secret Redis URL serialization through logging.
func (PublicSearch) String() string { return "public_search=[REDACTED]" }

type rawPublicSearch struct {
	RedisURL           string        `mapstructure:"redis_url"`
	Prefix             string        `mapstructure:"prefix"`
	Timeout            time.Duration `mapstructure:"timeout"`
	TokenTTL           time.Duration `mapstructure:"token_ttl"`
	QueryMaxChars      int           `mapstructure:"query_max_chars"`
	MaxResults         int           `mapstructure:"max_results"`
	SearchRate         int           `mapstructure:"search_rate"`
	SearchBurst        int           `mapstructure:"search_burst"`
	SessionRate        int           `mapstructure:"session_rate"`
	SessionBurst       int           `mapstructure:"session_burst"`
	ReadRate           int           `mapstructure:"read_rate"`
	ReadBurst          int           `mapstructure:"read_burst"`
	SSEConnections     int           `mapstructure:"sse_connections"`
	LeaseTTL           time.Duration `mapstructure:"lease_ttl"`
	LeaseRenewInterval time.Duration `mapstructure:"lease_renew_interval"`
}

func validatePublicSearch(raw rawPublicSearch) (PublicSearch, error) {
	parsed, err := url.Parse(raw.RedisURL)
	if err != nil || (parsed.Scheme != "redis" && parsed.Scheme != "rediss") || parsed.Host == "" {
		return PublicSearch{}, invalid("public_search.redis_url", "需要有效 Redis URL")
	}
	if strings.TrimSpace(raw.Prefix) == "" || strings.ContainsAny(raw.Prefix, "{} \t\r\n") {
		return PublicSearch{}, invalid("public_search.prefix", "需要无空白或花括号的环境前缀")
	}
	if raw.Timeout <= 0 || raw.TokenTTL < time.Second || raw.LeaseTTL < time.Millisecond || raw.LeaseRenewInterval < time.Millisecond || raw.LeaseRenewInterval >= raw.LeaseTTL {
		return PublicSearch{}, invalid("public_search", "超时必须为正，凭证至少一秒，租约/续租至少一毫秒且续租间隔小于租约期限")
	}
	if raw.QueryMaxChars < 1 || raw.QueryMaxChars > 16000 || raw.MaxResults < 1 || raw.MaxResults > 20 {
		return PublicSearch{}, invalid("public_search", "问题/结果限制超出 SearchInput 合同")
	}
	if raw.SearchRate < 1 || raw.SearchBurst < 1 || raw.SessionRate < 1 || raw.SessionBurst < 1 || raw.ReadRate < 1 || raw.ReadBurst < 1 || raw.SSEConnections < 1 {
		return PublicSearch{}, invalid("public_search", "频率、突发和连接数必须为正")
	}
	return PublicSearch{raw: raw}, nil
}
