package bad

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/bad/internal/domain"

// Handler demonstrates an external import of module internals.
type Handler struct{ Article domain.Outward }
