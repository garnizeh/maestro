package main

import "github.com/rodrigo-baliza/maestro/internal/cli"

// main is the program entry point and delegates control to the internal CLI package for command dispatch and execution.
func main() {
	cli.Execute()
}
