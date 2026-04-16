//go:build !amd64 && !arm64

package counter

// Read is a stub for architectures without a known fast CPU counter.
func Read() uint64 { return 0 }

// Supported returns false on unsupported architectures.
func Supported() bool { return false }
