package image

// imageOwnerProjectionRow is the read-only owner projection used by image persistence.
type imageOwnerProjectionRow struct {
	OwnerID string `gorm:"column:owner_id"`
}
