package rollup

import "testing"

func Test_Rollup(t *testing.T) {
	app := NewApplication()
	err := app.Start()
	if err != nil {
		panic(err)
	}
}
