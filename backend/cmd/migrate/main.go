// Command migrate 管理内嵌 PostgreSQL 数据库结构。
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

// main 执行迁移命令并只向终端输出无敏感值的中文错误。
func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "迁移命令执行失败："+err.Error())
		os.Exit(1)
	}
}

// run 从 YAML 读取数据库连接并执行一个迁移动作。
func run(args []string) (err error) {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Print("用法：migrate [-config YAML路径] <up|down|version|force VERSION>\n数据库连接读取 YAML 的 database.dsn；默认 config.local.yaml。\n")
		return flag.ErrHelp
	}
	dsn, command, err := loadMigrationConfig(args)
	if err != nil {
		return err
	}
	if len(command) == 0 {
		return fmt.Errorf("缺少迁移命令：up、down、version 或 force")
	}
	if command[0] == "force" && len(command) != 2 {
		return fmt.Errorf("force 命令必须提供版本号")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("打开数据库失败：%w", err)
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	if pingErr := db.PingContext(context.Background()); pingErr != nil {
		return fmt.Errorf("连接数据库失败")
	}
	runner, err := migrations.New(db, "")
	if err != nil {
		return fmt.Errorf("创建迁移执行器失败：%w", err)
	}
	defer func() { err = errors.Join(err, runner.Close()) }()
	switch command[0] {
	case "up":
		return runner.Up()
	case "down":
		return runner.Down()
	case "version":
		version, dirty, versionErr := runner.Version()
		if versionErr == nil {
			fmt.Printf("%d dirty=%t\n", version, dirty)
		}
		return versionErr
	case "force":
		version, parseErr := strconv.Atoi(command[1])
		if parseErr != nil {
			return fmt.Errorf("版本号格式无效")
		}
		return runner.Force(version)
	default:
		return fmt.Errorf("未知迁移命令：%s", command[0])
	}
}
