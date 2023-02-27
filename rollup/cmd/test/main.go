package main

import (
	sdk "github.com/argus-labs/we-sdk"
)

func main() {
	cfg, err := sdk.LoadConfig("example")
	if err != nil {
		panic(err)
	}
	app := sdk.NewApplication(cfg)
	err = app.Start()
	if err != nil {
		panic(err)
	}
}
