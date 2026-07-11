package domain

import "gorm.io/gorm"

// Record demonstrates forbidden persistence leakage.
type Record struct{ DB *gorm.DB }
