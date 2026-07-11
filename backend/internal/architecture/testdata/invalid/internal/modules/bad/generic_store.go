package bad

// Store demonstrates name-independent generic interface rejection.
type Store[T any] interface{ Put(T) error }
