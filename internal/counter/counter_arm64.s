#include "textflag.h"

// func Read() uint64
// Reads the ARM generic timer virtual count register (CNTVCT_EL0).
TEXT ·Read(SB), NOSPLIT, $0-8
	WORD $0xd53be040 // MRS CNTVCT_EL0, X0
	MOVD R0, ret+0(FP)
	RET
