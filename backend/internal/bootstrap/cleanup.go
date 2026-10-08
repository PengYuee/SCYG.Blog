package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

type cleanupStep struct {
	name  string
	close func(context.Context) error
}

type cleanupStack []cleanupStep

// Close 按资源创建逆序执行全部清理，并保留根错误与每个关闭错误。
func (stack cleanupStack) Close(ctx context.Context, root error) error {
	result := root
	for index := len(stack) - 1; index >= 0; index-- {
		step := stack[index]
		if err := step.close(ctx); err != nil {
			result = errors.Join(result, fmt.Errorf("关闭%s: %w", step.name, err))
		}
	}
	return result
}

// nilLike 安全识别 nil 接口及所有可空动态类型的 typed-nil。
func nilLike(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() { //nolint:exhaustive // only nullable kinds can be typed-nil.
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func validateDependencies(dependencies Dependencies) error {
	factories := []struct {
		name  string
		value any
	}{
		{"配置加载器", dependencies.LoadConfig},
		{"日志构造器", dependencies.NewLogger},
		{"遥测构造器", dependencies.NewTelemetry},
		{"数据库构造器", dependencies.NewDatabase},
		{"迁移构造器", dependencies.NewMigration},
		{"文章 feature 构造器", dependencies.NewArticle},
		{"taxonomy feature 构造器", dependencies.NewTaxonomy},
		{"图片 feature 构造器", dependencies.NewImage},
		{"文章图片协作构造器", dependencies.NewArticleImages},
		{"文章响应协作构造器", dependencies.NewArticleResponses},
		{"图片清理构造器", dependencies.NewImageCleanup},
		{"图片清理 worker 构造器", dependencies.NewCleanupWorker},
		{"REST 构造器", dependencies.NewREST},
		{"HTTP 构造器", dependencies.NewHTTP},
	}
	for _, factory := range factories {
		if nilLike(factory.value) {
			return fmt.Errorf("%s为空", factory.name)
		}
	}
	return nil
}
