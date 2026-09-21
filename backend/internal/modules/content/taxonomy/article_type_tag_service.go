package taxonomy

import (
	"errors"
	"reflect"
	"time"

	"gorm.io/gorm"

	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
)

// Service owns taxonomy business behavior without depending on another feature.
type Service struct {
	repo       *Repository
	clock      content.Clock
	authorizer content.Authorizer
}

// New constructs taxonomy from a GORM handle and explicit collaborators.
func New(db *gorm.DB, dependencies Dependencies) (*Service, error) {
	repo, err := NewRepository(db)
	if err != nil {
		return nil, err
	}
	return NewService(repo, dependencies)
}

// NewService constructs taxonomy from its concrete repository.
func NewService(repo *Repository, dependencies Dependencies) (*Service, error) {
	if repo == nil || repo.db == nil {
		return nil, errors.New("taxonomy repository is nil")
	}
	if nilLikeClock(dependencies.Clock) {
		return nil, errors.New("taxonomy clock is nil")
	}
	return &Service{repo: repo, clock: dependencies.Clock, authorizer: content.AuthorizerOrDeny(dependencies.Authorizer)}, nil
}

func (service *Service) now(floor time.Time) (time.Time, error) {
	if service.clock == nil {
		return time.Time{}, internalWithCause(errors.New("taxonomy clock is nil"))
	}
	now := service.clock.Now().UTC()
	if now.IsZero() || now.Before(floor) {
		return time.Time{}, internalWithCause(errors.New("taxonomy clock is invalid"))
	}
	return now, nil
}

func nilLikeClock(clock content.Clock) bool {
	if clock == nil {
		return true
	}
	value := reflect.ValueOf(clock)
	switch value.Kind() { //nolint:exhaustive // only nullable kinds can be typed-nil.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
