package argus

import (
	"testing"

	"github.com/spf13/cast"
	"gotest.tools/assert"
)

func Test_Config(t *testing.T) {
	cpuProf := "test"
	cfg := AppConfig{CPUProfile: cpuProf}
	gotCpuProf := cfg.Get("cpu-profile")
	cpuProfStr := cast.ToString(gotCpuProf)
	assert.Equal(t, cpuProf, cpuProfStr)
}
