package domain

import "time"

func taxonomyCurrent(actual, expected Version, deletedAt time.Time) error {
	if !expected.valid() || actual != expected {
		return &VersionConflict{Expected: expected, Actual: actual}
	}
	if !deletedAt.IsZero() {
		return ErrDeleted
	}
	return nil
}
