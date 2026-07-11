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
	switch reflected.Kind() {
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
		{"配置加载器", dependencies.LoadConfig}, {"日志构造器", dependencies.NewLogger},
		{"遥测构造器", dependencies.NewTelemetry}, {"数据库构造器", dependencies.NewDatabase},
		{"迁移构造器", dependencies.NewMigration}, {"内容构造器", dependencies.NewContent},
		{"REST 构造器", dependencies.NewREST}, {"HTTP 构造器", dependencies.NewHTTP},
	}
	for _, factory := range factories {
		if nilLike(factory.value) {
			return fmt.Errorf("%s为空", factory.name)
		}
	}
	return nil
}
