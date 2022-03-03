#!/usr/bin/env bash

# This script checks if a binary program (EXE) compiled for
# Darwin (macOS) links to __getdirentries64.

if [[ -t 2 ]]; then
    ERROR=$'\E[00;31m'error$'\E[0m'
else
    ERROR=error
fi

[[ ! -r "$1" ]] && {
    echo -e >&2 "${ERROR} invalid argument EXE: no such file or directory: $1"
    echo >&2 "usage: $0 EXE"
    exit 1
}
[[ "$(uname -s)" != Darwin ]] && {
    echo -e >&2 "${ERROR} invalid OS: $(uname -s)"
    echo >&2 ''
    echo >&2 "This script is only supported on and to applicable to Darwin (macOS)."
    exit 2
}
! otool -dyld_info "$1" | \grep -qF getdirentries64 || {
    echo -e >&2 "${ERROR} executable links to __getdirentries64: $1"
    echo >&2 ''
    echo >&2 'To not link to __getdirentries64 use the "nogetdirentries" build tag:'
    echo >&2 '  $ go build -tags nogetdirentries <YOUR_EXE>'
    exit 2
}
