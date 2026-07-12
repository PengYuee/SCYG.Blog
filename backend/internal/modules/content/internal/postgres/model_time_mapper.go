package postgres

import "time"

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copied := value.UTC()
	return &copied
}
func timeValue(value *time.Time, fallback time.Time) time.Time {
	if value == nil {
		return fallback
	}
	return value.UTC()
}
