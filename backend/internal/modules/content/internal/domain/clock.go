package domain

import (
	"reflect"
	"time"
)

func clockTime(clock Clock, floor time.Time) (time.Time, error) {
	if clock == nil {
		return time.Time{}, ErrInvalidClock
	}
	value := reflect.ValueOf(clock)
	if (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) && value.IsNil() {
		return time.Time{}, ErrInvalidClock
	}
	now := clock.Now().UTC()
	if now.IsZero() {
		return time.Time{}, ErrInvalidClock
	}
	if now.Before(floor) {
		return time.Time{}, ErrTimeRegression
	}
	return now, nil
}
