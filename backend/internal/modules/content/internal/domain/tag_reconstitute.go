package domain

// ReconstituteTag 通过领域不变量恢复持久化标签。
func ReconstituteTag(state TaxonomyState[TagID]) (*Tag, error) {
	if !state.ID.valid() || !state.Name.valid() || !state.Version.valid() || state.CreatedAt.IsZero() || state.ModifiedAt.Before(state.CreatedAt) {
		return nil, ErrInvalidValue
	}
	return &Tag{id: state.ID, name: state.Name, version: state.Version, createdAt: state.CreatedAt, modifiedAt: state.ModifiedAt, deletedAt: state.DeletedAt}, nil
}
