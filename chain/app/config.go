package argus

import (
	"reflect"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/server/types"
)

type AppConfig struct {
	CPUProfile                  string        `toml:"cpu-profile"`
	DbBackend                   string        `toml:"db-backend"`
	TraceStore                  string        `toml:"trace-store"`
	GrpcOnly                    bool          `toml:"grpc-only"`
	InterBlockCache             bool          `toml:"inter-block-cache"`
	UnsafeSkipUpgrades          int           `toml:"unsafe-skip-upgrades"`
	Home                        string        `toml:"home"`
	InvCheckPeriod              int           `toml:"inv-check-period"`
	MinGasPrices                string        `toml:"min-gas-prices"`
	HaltHeight                  int           `toml:"halt-height"`
	HaltTime                    time.Time     `toml:"halt-time"`
	MinRetainBlocks             int           `toml:"min-retain-blocks"`
	Trace                       bool          `toml:"trace"`
	IndexEvents                 []interface{} `toml:"index-events"`
	IAVLCacheSize               int           `toml:"IAVL-cache-size"`
	DisableIVALFastNode         bool          `toml:"disable-IVAL-fast-node"`
	XCrisisSkipAssertInvariants bool          `toml:"x-crisis-skip-assert-invariants"`
	EvmMaxTxGasWanted           int           `toml:"evm-max-tx-gas-wanted"`
	EvmTracer                   string        `toml:"evm-tracer"`
	SnapshotInterval            int           `toml:"state-sync.snapshot-interval"`
	SnapshotKeepRecent          int           `toml:"state-sync.snapshot-keep-recent"`
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
