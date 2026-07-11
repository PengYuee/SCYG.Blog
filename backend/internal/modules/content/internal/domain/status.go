package domain

import "fmt"

// Status is the closed article lifecycle state.
type Status string

const (
	// StatusDraft permits revision and publication.
	StatusDraft Status = "draft"
	// StatusPublished permits revision and archival.
	StatusPublished Status = "published"
	// StatusArchived is terminal for content revision.
	StatusArchived Status = "archived"
)

// ParseStatus parses an external status string into the closed enum.
func ParseStatus(raw string) (Status, error) {
	switch Status(raw) {
	case StatusDraft:
		return StatusDraft, nil
	case StatusPublished:
		return StatusPublished, nil
	case StatusArchived:
		return StatusArchived, nil
	default:
		return "", fmt.Errorf("status: %w", ErrInvalidValue)
	}
}
