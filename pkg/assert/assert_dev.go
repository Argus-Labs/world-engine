//go:build !release

package assert

import "fmt"

func That(cond bool, format string, args ...any) { //nolint:goprintffuncname // it's ok
	if !cond {
		panic(fmt.Sprintf(format, args...))
	}
}
