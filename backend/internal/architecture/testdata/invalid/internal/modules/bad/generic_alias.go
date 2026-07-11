package bad

// Port demonstrates a generic interface alias bypass.
type Port[T any] = interface{ Put(T) error }
