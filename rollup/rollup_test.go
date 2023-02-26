package rollup

import (
	"testing"
)

func Test_Rollup(t *testing.T) {
	cfg := DefaultConfig()
	cfg.appCfg.MinGasPrices = "1stake"
	cfg.sCfg.MinGasPrices = "1stake"
	cfg.sCtx.Config.BaseConfig.Genesis = "someGenesis.json"
	cfg.rollCfg.DALayer = "mock"
	app := NewApplication(&cfg)
	err := app.Start()
	if err != nil {
		panic(err)
	}
}
