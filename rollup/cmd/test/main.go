package main

import (
	"github.com/argus-labs/we-sdk/kit"
)

func main() {
	cfg, err := kit.LoadConfig("example")
	if err != nil {
		panic(err)
	}
	app := kit.NewApplication(cfg)
	err = app.Start()
	if err != nil {
		panic(err)
	}
}
