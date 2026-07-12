package content

// CreateTag 描述创建标签的输入。
type CreateTag struct {
	// Name 是标签名称。
	Name string
}

// RenameTag 描述基于版本重命名标签的命令。
type RenameTag struct {
	// ID 是待重命名标签 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
	// Name 是新的标签名称。
	Name string
}

// DeleteTag 描述基于版本软删除标签的命令。
type DeleteTag struct {
	// ID 是待删除标签 ID。
	ID int64
	// Version 是乐观锁版本。
	Version uint64
}
