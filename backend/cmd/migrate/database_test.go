package main

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Test_parseDatabaseName_严格解析目标库名验证 PostgreSQL URL 边界。
func Test_parseDatabaseName_严格解析目标库名(t *testing.T) {
	tests := []struct {
		name    string
		dsn     string
		want    string
		wantErr bool
	}{
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "解码单段库名", dsn: "postgres://user:secret@localhost:5432/scyg%5Fblog?sslmode=disable", want: "scyg_blog"},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "允许查询参数", dsn: "postgresql://user:secret@localhost/blog?application_name=migrate", want: "blog"},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝空库名", dsn: "postgres://user:secret@localhost/", wantErr: true},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝多段路径", dsn: "postgres://user:secret@localhost/one/two", wantErr: true},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝编码斜杠", dsn: "postgres://user:secret@localhost/one%2Ftwo", wantErr: true},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝空字节", dsn: "postgres://user:secret@localhost/one%00two", wantErr: true},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝片段", dsn: "postgres://user:secret@localhost/blog#fragment", wantErr: true},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝畸形转义", dsn: "postgres://user:secret@localhost/bad%zz", wantErr: true},
		//nolint:gosec // synthetic DSN used only to test database-name parsing.
		{name: "拒绝非PostgreSQL", dsn: "mysql://user:secret@localhost/blog", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got, err := parseDatabaseName(test.dsn)

			// Then
			if test.wantErr {
				if err == nil || strings.Contains(err.Error(), "secret") {
					t.Fatalf("应返回脱敏错误，got=%q err=%v", got, err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("got=%q want=%q err=%v", got, test.want, err)
			}
		})
	}
}

// Test_parseAdminDSN_只接受postgres管理库验证建库连接不会指向业务库。
func Test_parseAdminDSN_只接受postgres管理库(t *testing.T) {
	// Given / When
	_, err := parseAdminDSN("postgres://admin:secret@localhost:5432/other?sslmode=disable")

	// Then
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("应拒绝非 postgres 管理库且不泄露密码：%v", err)
	}
}

// Test_isDuplicateDatabase_仅接受42P04验证并发创建幂等分支。
func Test_isDuplicateDatabase_仅接受42P04(t *testing.T) {
	// Given
	duplicate := &pgconn.PgError{Code: "42P04"}
	unavailable := &pgconn.PgError{Code: "3D000"}

	// When / Then
	if !isDuplicateDatabase(duplicate) {
		t.Fatal("42P04 应视为并发创建成功")
	}
	if isDuplicateDatabase(unavailable) {
		t.Fatal("非 42P04 不应视为并发创建成功")
	}
}

// Test_createDatabaseSQL_安全引用标识符验证特殊字符不会形成原始 SQL。
func Test_createDatabaseSQL_安全引用标识符(t *testing.T) {
	// Given
	name := `qa_"quoted`

	// When
	got := createDatabaseSQL(name)

	// Then
	want := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize()
	if got != want || strings.Contains(got, `qa_"quoted`) {
		t.Fatalf("got=%q want=%q", got, want)
	}
}
