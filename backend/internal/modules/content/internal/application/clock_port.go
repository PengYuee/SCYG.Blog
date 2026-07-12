// Package application 定义内容用例端口和事务边界。
package application

import "time"

// Clock 为应用层提供确定性时间。
type Clock interface{ Now() time.Time }
