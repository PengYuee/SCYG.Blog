package user

import "fmt"

func userFromRecord(row userRecord) (*User, error) {
	id, err := ParseID(row.ID)
	if err != nil {
		return nil, fmt.Errorf("map user id: %w", err)
	}
	value := User{ID: id, Username: row.Username, passwordHash: row.PasswordHash, Active: row.IsActive, CreatedAt: row.CreatedAt.UTC(), UpdatedAt: row.UpdatedAt}
	if value.UpdatedAt != nil {
		updated := value.UpdatedAt.UTC()
		value.UpdatedAt = &updated
	}
	if err := validateUser(value); err != nil {
		return nil, fmt.Errorf("map user: %w", err)
	}
	return &value, nil
}
