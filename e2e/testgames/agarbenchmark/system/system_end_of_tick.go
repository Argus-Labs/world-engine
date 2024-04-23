// NOTE: This is currently unused and can be safely removed. I'm leaving this here for reference only.
package system

import "pkg.world.dev/world-engine/cardinal"

type EndOfTickFunction func()

var endOfTickFunctions []EndOfTickFunction

func EndOfTickSystem(_ cardinal.WorldContext) error {
	for _, f := range endOfTickFunctions {
		f()
	}
	// Clear the slice after executing all EOT functions.
	endOfTickFunctions = endOfTickFunctions[:0]
	return nil
}

func ScheduleEndOfTickFunction(f EndOfTickFunction) {
	endOfTickFunctions = append(endOfTickFunctions, f)
}
