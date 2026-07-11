module github.com/PengYuee/SCYG.Blog/backend

go 1.25.0

require (
	github.com/go-viper/mapstructure/v2 v2.2.1
	github.com/prometheus/client_golang v1.23.2
	github.com/spf13/viper v1.20.1
	go.opentelemetry.io/otel/trace v1.44.0
)

require github.com/rogpeppe/go-internal v1.14.1 // indirect

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/fsnotify/fsnotify v1.8.0 // indirect
	github.com/gin-gonic/gin v1.11.0
	github.com/jackc/pgx/v5 v5.10.0
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/sagikazarmark/locafero v0.7.0 // indirect
	github.com/sourcegraph/conc v0.3.0 // indirect
	github.com/spf13/afero v1.12.0 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/testcontainers/testcontainers-go v0.43.0
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/goleak v1.3.0
	go.uber.org/multierr v1.9.0 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/protobuf v1.36.9 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gorm.io/driver/postgres v1.6.0
	gorm.io/gorm v1.31.1
)

replace github.com/spf13/afero v1.12.0 => github.com/spf13/afero v1.6.0
