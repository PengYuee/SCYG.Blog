package bootstrap

import (
	"context"
	"errors"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	outbound "github.com/PengYuee/SCYG.Blog/backend/internal/adapters/agent"
	blogpb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/blog/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/application"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/article"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/operation"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/taxonomy"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/grpc/blogcontent"
)

type agentIntegration struct {
	client   *outbound.Client
	server   *grpc.Server
	health   *health.Server
	listen   string
	errors   <-chan error
	listener net.Listener
}

func newAgentIntegration(cfg config.Agent, db Database, articles *article.Service, taxonomyService *taxonomy.Service, articleImages *application.ArticleImages) (*agentIntegration, *operation.Service, error) {
	ledger, err := operation.New(db.GORM())
	if err != nil {
		return nil, nil, err
	}
	operations, err := application.NewArticleOperations(application.OperationDependencies{DB: db.GORM(), Articles: articles, ArticleImages: articleImages, Operations: ledger})
	if err != nil {
		return nil, nil, err
	}
	users, err := user.NewRepository(db.GORM())
	if err != nil {
		return nil, nil, err
	}
	client, err := outbound.New(cfg.Target(), cfg.UnaryTimeout())
	if err != nil {
		return nil, nil, err
	}
	server := grpc.NewServer()
	healthServer, err := blogcontent.Register(server, blogcontent.Dependencies{Articles: articles, Taxonomy: taxonomyService, Operations: operations, Users: users})
	if err != nil {
		return nil, nil, errors.Join(err, client.Close())
	}
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	return &agentIntegration{client: client, server: server, health: healthServer, listen: cfg.BlogContentListen()}, ledger, nil
}

func (a *agentIntegration) start() error {
	listener, err := net.Listen("tcp", a.listen)
	if err != nil {
		return err
	}
	a.listener = listener
	result := make(chan error, 1)
	a.errors = result
	go func() {
		err := a.server.Serve(listener)
		if errors.Is(err, grpc.ErrServerStopped) {
			err = nil
		}
		result <- err
		close(result)
	}()
	return nil
}

func (a *agentIntegration) stop(timeout time.Duration) {
	a.health.Shutdown()
	done := make(chan struct{})
	go func() { a.server.GracefulStop(); close(done) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		a.server.Stop()
	}
}

type combinedCleanup struct {
	images     CleanupRunner
	operations *operation.Service
}

func (c combinedCleanup) CleanupArticleImages(ctx context.Context) error {
	_, err := c.operations.CleanupExpired(ctx, 1000)
	return errors.Join(err, c.images.CleanupArticleImages(ctx))
}

func (a *agentIntegration) activate() {
	a.health.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	a.health.SetServingStatus(blogpb.BlogContentService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)
}
