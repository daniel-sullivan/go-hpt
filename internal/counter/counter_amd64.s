#include "textflag.h"

// func Read() uint64
// Reads the CPU Time Stamp Counter via RDTSCP (serializing variant).
TEXT ·Read(SB), NOSPLIT, $0-8
	BYTE $0x0F; BYTE $0x01; BYTE $0xF9 // RDTSCP: EDX:EAX = TSC, ECX = core ID
	SHLQ $32, DX
	ORQ  DX, AX
	MOVQ AX, ret+0(FP)
	RET

// func Supported() bool
// Returns true if the CPU has both RDTSCP and invariant TSC support.
TEXT ·Supported(SB), NOSPLIT, $0-1
	// Check max extended CPUID leaf.
	MOVL $0x80000000, AX
	CPUID
	CMPL AX, $0x80000007
	JB   unsupported

	// Check RDTSCP: CPUID 0x80000001, EDX bit 27.
	MOVL $0x80000001, AX
	CPUID
	TESTL $(1<<27), DX
	JZ   unsupported

	// Check invariant TSC: CPUID 0x80000007, EDX bit 8.
	MOVL $0x80000007, AX
	CPUID
	TESTL $(1<<8), DX
	JZ   unsupported

	MOVB $1, ret+0(FP)
	RET

unsupported:
	MOVB $0, ret+0(FP)
	RET
