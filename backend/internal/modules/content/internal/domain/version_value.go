package domain

import "math"

// Version 是严格为正的乐观并发版本。
type Version struct{ value uint64 }

// NewVersion 解析严格为正的版本号。
func NewVersion(raw uint64) (Version, error) {
	if raw == 0 {
		return Version{}, invalid("version")
	}
	return Version{raw}, nil
}
func initialVersion() Version { return Version{1} }
func (version Version) next() (Version, error) {
	if version.value == math.MaxUint64 {
		return Version{}, ErrVersionExhausted
	}
	return Version{version.value + 1}, nil
}

// Uint64 返回乐观并发版本号。
func (version Version) Uint64() uint64 { return version.value }
func (version Version) valid() bool    { return version.value > 0 }
