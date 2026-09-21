package taxonomy

import "time"

func copyImage(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func nullableTimeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}
