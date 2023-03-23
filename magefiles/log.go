package main

import (
	"fmt"
)

// Color constants for terminal output.
const (
	resetColor   = "\033[0m"
	redColor     = "\033[31m"
	greenColor   = "\033[32m"
	yellowColor  = "\033[33m"
	blueColor    = "\033[34m"
	magentaColor = "\033[35m"
	cyanColor    = "\033[36m"
)

// LogRed prints a log message with red color.
func LogRed(msg string, args ...interface{}) {
	log(redColor, msg, args...)
}

// LogGreen prints a log message with green color.
func LogGreen(msg string, args ...interface{}) {
	log(greenColor, msg, args...)
}

// LogYellow prints a log message with yellow color.
func LogYellow(msg string, args ...interface{}) {
	log(yellowColor, msg, args...)
}

// LogBlue prints a log message with blue color.
func LogBlue(msg string, args ...interface{}) {
	log(blueColor, msg, args...)
}

// LogMagenta prints a log message with magenta color.
func LogMagenta(msg string, args ...interface{}) {
	log(magentaColor, msg, args...)
}

// LogCyan prints a log message with cyan color.
func LogCyan(msg string, args ...interface{}) {
	log(cyanColor, msg, args...)
}

// log is a helper function that prints a log message and key-value pairs
// with a specified color.
// colorCode: ANSI escape code for the desired color
// msg: The log message to be printed
// args: Key-value pairs to be printed
func log(colorCode string, msg string, args ...interface{}) {
	fmt.Printf("%s%s%s\n", colorCode, msg, resetColor)
	for i := 0; i < len(args); i += 2 {
		key := args[i]
		value := args[i+1]
		fmt.Printf("%s  %v: %v%s\n", colorCode, key, value, resetColor)
	}
}
