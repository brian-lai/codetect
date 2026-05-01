package main

import (
	"os"

	"codetect/cmd/codetect/commands"
)

func main() {
	os.Exit(int(commands.Dispatch(os.Args[1:])))
}
