package domain

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/bad/internal/application"

// Outward demonstrates an illegal dependency direction.
type Outward struct{ Port application.Port }
