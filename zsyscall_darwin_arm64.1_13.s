//go:build go1.13
// +build go1.13

#include "textflag.h"

TEXT libc_opendir_trampoline<>(SB),NOSPLIT,$0-0
	JMP	libc_opendir(SB)

GLOBL	·libc_opendir_trampoline_addr(SB), RODATA, $8
DATA	·libc_opendir_trampoline_addr(SB)/8, $libc_opendir_trampoline<>(SB)

TEXT libc___getdirentries64_trampoline<>(SB),NOSPLIT,$0-0
	JMP libc___getdirentries64(SB)

GLOBL	·libc___getdirentries64_trampoline_addr(SB), RODATA, $8
DATA	·libc___getdirentries64_trampoline_addr(SB)/8, $libc___getdirentries64_trampoline<>(SB)
