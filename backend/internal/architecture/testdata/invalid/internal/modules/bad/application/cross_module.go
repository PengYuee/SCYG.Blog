package application

import "github.com/PengYuee/SCYG.Blog/backend/internal/modules/other/article"

type CrossModuleDependency struct{ Value article.Service }
