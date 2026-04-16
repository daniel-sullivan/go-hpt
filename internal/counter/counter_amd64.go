package counter

// Read reads the x86 TSC via RDTSCP.
func Read() uint64

// Supported returns true when RDTSCP and invariant TSC are both present.
func Supported() bool
