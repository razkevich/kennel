#!/bin/sh
# Runs the Swift tests. With only Command Line Tools (no Xcode), swift-testing lives
# in a framework directory the compiler doesn't search by default.
set -e
cd "$(dirname "$0")"
F=/Library/Developer/CommandLineTools/Library/Developer/Frameworks
if [ -d "$F/Testing.framework" ] && ! xcode-select -p | grep -q Xcode.app; then
  exec swift test -Xswiftc -F$F -Xlinker -F$F -Xlinker -rpath -Xlinker $F \
    -Xlinker -rpath -Xlinker /Library/Developer/CommandLineTools/Library/Developer/usr/lib "$@"
fi
exec swift test "$@"
