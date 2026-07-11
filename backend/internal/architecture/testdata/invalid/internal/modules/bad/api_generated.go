package bad

import "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"

// LeakedResult demonstrates generated transport leakage.
type LeakedResult struct{ Value openapi.Article }
