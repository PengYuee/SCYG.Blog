package bad

// Repository demonstrates a forbidden generic repository abstraction.
type Repository[T any] interface{ Save(T) error }
