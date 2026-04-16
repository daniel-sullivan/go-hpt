package counter

// Read reads the ARM64 virtual counter (CNTVCT_EL0).
func Read() uint64

// Supported reports whether direct counter reads are available.
// ARM64 always exposes CNTVCT_EL0 to userspace.
func Supported() bool { return true }
