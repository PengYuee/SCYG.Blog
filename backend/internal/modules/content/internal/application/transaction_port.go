package application

import "context"

// Transaction 仅暴露事务范围内的内容端口，仓储不得跨该范围泄露。
type Transaction interface {
	Articles() ArticleRepository
	ArticleTypes() ArticleTypeRepository
	Tags() TagRepository
	ArticleImages() ArticleImageRepository
}

// UnitOfWork 在不泄露事务框架类型的前提下原子执行回调；成功才提交，错误必须回滚。
type UnitOfWork interface {
	Within(context.Context, func(context.Context, Transaction) error) error
}
