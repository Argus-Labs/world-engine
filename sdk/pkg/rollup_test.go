package pkg

import (
	"testing"
)

func Test_Rollup(t *testing.T) {
	cfg, err := LoadConfig("example")
	if err != nil {
		panic(err)
	}
	app := NewApplication(cfg)
	err = app.Start()
	if err != nil {
		panic(err)
	}
}
