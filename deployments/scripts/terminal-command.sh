#!/bin/sh

"$@"
STATUS=$?
[ "$STATUS" -eq 0 ] && exit 0
printf '\nCommand failed. Press Enter to close.\n' >&2
read -r _
exit "$STATUS"
