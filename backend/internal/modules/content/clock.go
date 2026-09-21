// Package content defines shared content capabilities and policies.
package content

import "time"

// Clock supplies deterministic UTC time to content use cases.
type Clock interface {
	Now() time.Time
}
