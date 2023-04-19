package argus

import (
	"reflect"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/server/types"
)

type AppConfig struct {
	Genesis                     string        `mapstructure:"genesis"`
	CPUProfile                  string        `mapstructure:"cpu-profile"`
	DbBackend                   string        `mapstructure:"db-backend"`
	TraceStore                  string        `mapstructure:"trace-store"`
	GrpcOnly                    bool          `mapstructure:"grpc-only"`
	InterBlockCache             bool          `mapstructure:"inter-block-cache"`
	UnsafeSkipUpgrades          int           `mapstructure:"unsafe-skip-upgrades"`
	Home                        string        `mapstructure:"home"`
	InvCheckPeriod              int           `mapstructure:"inv-check-period"`
	MinGasPrices                string        `mapstructure:"min-gas-prices"`
	HaltHeight                  int           `mapstructure:"halt-height"`
	HaltTime                    time.Time     `mapstructure:"halt-time"`
	MinRetainBlocks             int           `mapstructure:"min-retain-blocks"`
	Trace                       bool          `mapstructure:"trace"`
	IndexEvents                 []interface{} `mapstructure:"index-events"`
	IAVLCacheSize               int           `mapstructure:"IAVL-cache-size"`
	DisableIVALFastNode         bool          `mapstructure:"disable-IVAL-fast-node"`
	XCrisisSkipAssertInvariants bool          `mapstructure:"x-crisis-skip-assert-invariants"`
	EvmMaxTxGasWanted           int           `mapstructure:"evm-max-tx-gas-wanted"`
	EvmTracer                   string        `mapstructure:"evm-tracer"`
	SnapshotInterval            int           `mapstructure:"state-sync.snapshot-interval"`
	SnapshotKeepRecent          int           `mapstructure:"state-sync.snapshot-keep-recent"`
}

func (a AppConfig) Get(s string) interface{} {
	tag := "toml"
	fieldName := getFieldName(tag, s, a)
	return getField(&a, fieldName)
}

func getField(v *AppConfig, field string) interface{} {
	r := reflect.ValueOf(v)
	f := reflect.Indirect(r).FieldByName(field)
	if !f.IsValid() {
		return nil
	}
	return f.Interface()
}

func getFieldName(tag, key string, s AppConfig) (fieldName string) {
	rt := reflect.TypeOf(s)
	if rt.Kind() != reflect.Struct {
		panic("bad type")
	}
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		sTag := f.Tag.Get(tag)
		v := strings.Split(sTag, ",")[0] // use split to ignore tag "options" like omitempty, etc.
		if v == key {
			return f.Name
		}
	}
	return ""
}

var _ types.AppOptions = AppConfig{}
