package image

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// Service executes image commands and coordinates image persistence and Blob I/O.
type Service struct {
	db            *gorm.DB
	repository    *Repository
	clock         content.Clock
	authorizer    content.Authorizer
	currentAuthor content.CurrentAuthorProvider
	blob          Blob
	policy        Policy
}

// New constructs an image service.
func New(db *gorm.DB, dependencies Dependencies) (*Service, error) {
	if db == nil {
		return nil, errors.New("image database is nil")
	}
	if dependencies.Clock == nil {
		return nil, errors.New("image clock is nil")
	}
	if dependencies.Blob == nil {
		return nil, errors.New("image blob is nil")
	}
	repository, err := NewRepository(db)
	if err != nil {
		return nil, err
	}
	return &Service{db: db, repository: repository, authorizer: content.AuthorizerOrDeny(dependencies.Authorizer), currentAuthor: content.CurrentAuthorProviderOrUnavailable(dependencies.CurrentAuthor), clock: dependencies.Clock, blob: dependencies.Blob, policy: dependencies.Policy.valid()}, nil
}

func (service *Service) now() time.Time {
	if service.clock == nil {
		return time.Now().UTC()
	}
	return service.clock.Now().UTC()
}
