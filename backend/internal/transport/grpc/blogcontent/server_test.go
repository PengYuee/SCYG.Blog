package blogcontent

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
)

type testAccounts struct{ account *user.User }

func (accounts testAccounts) FindActiveByID(_ context.Context, id user.ID) (*user.User, error) {
	if accounts.account == nil || accounts.account.ID != id {
		return nil, user.ErrNotFound
	}
	return accounts.account, nil
}

func TestWriteContextRejectsInvalidIdentityAndNonV4Keys(t *testing.T) {
	validUser := "abcdef0123456789abcdef0123456789"
	validKey := "12345678-1234-4234-8234-123456789abc"
	id, err := user.ParseID(validUser)
	if err != nil {
		t.Fatal(err)
	}
	account, err := user.Reconstitute(id, "validation-test", "stored-test-hash", true, time.Now().UTC(), nil)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{dependencies: Dependencies{Users: testAccounts{account: account}}}
	for _, key := range []string{"", "12345678-1234-1234-8234-123456789abc", "12345678-1234-4234-7234-123456789abc", "12345678-1234-4234-8234-123456789abz", validKey + "\n"} {
		if _, err := server.writeContext(context.Background(), validUser, key); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("key %q accepted: %v", key, err)
		}
	}
	if _, err := server.writeContext(context.Background(), "invalid", validKey); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid subject=%v", err)
	}
	if _, err := server.writeContext(context.Background(), "11111111111111111111111111111111", validKey); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing subject=%v", err)
	}
	account.Active = false
	if _, err := server.writeContext(context.Background(), validUser, validKey); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("inactive subject=%v", err)
	}
	account.Active = true
	ctx, err := server.writeContext(context.Background(), validUser, "12345678-1234-4234-A234-123456789ABC")
	if err != nil {
		t.Fatal(err)
	}
	principal, ok := identityauth.PrincipalFromContext(ctx)
	if !ok || principal.UserID.String() != validUser {
		t.Fatalf("business principal=%+v present=%t", principal, ok)
	}
}
