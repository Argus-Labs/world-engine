package dependency

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/suite"
)

// Helper to create a CmdFactory for tests.
func cmdFactory(name string, args ...string) CmdFactory {
	return func() *exec.Cmd { return exec.Command(name, args...) }
}

// =============================================================================
// DependencySuite - tests for dependency.go
// =============================================================================

type DependencySuite struct {
	suite.Suite
}

func TestDependencySuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DependencySuite))
}

func (s *DependencySuite) TestCheck() {
	tests := []struct {
		name        string
		dep         Dependency
		wantErr     bool
		errContains string
	}{
		{
			name:    "success",
			dep:     Dependency{Name: "true-cmd", CmdFactory: cmdFactory("true")},
			wantErr: false,
		},
		{
			name:        "failure",
			dep:         Dependency{Name: "false-cmd", CmdFactory: cmdFactory("false")},
			wantErr:     true,
			errContains: `dependency check for "false-cmd" failed`,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := tt.dep.Check()
			if tt.wantErr {
				s.Require().Error(err)
				s.Contains(err.Error(), tt.errContains)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

func (s *DependencySuite) TestCheckMultiple() {
	tests := []struct {
		name    string
		deps    []Dependency
		wantErr bool
	}{
		{
			name: "all success",
			deps: []Dependency{
				{Name: "ok1", CmdFactory: cmdFactory("true")},
				{Name: "ok2", CmdFactory: cmdFactory("true")},
			},
			wantErr: false,
		},
		{
			name: "mixed results",
			deps: []Dependency{
				{Name: "ok", CmdFactory: cmdFactory("true")},
				{Name: "fail", CmdFactory: cmdFactory("false")},
			},
			wantErr: true,
		},
		{
			name: "all fail",
			deps: []Dependency{
				{Name: "fail1", CmdFactory: cmdFactory("false")},
				{Name: "fail2", CmdFactory: cmdFactory("false")},
			},
			wantErr: true,
		},
		{
			name:    "empty",
			deps:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			err := Check(tt.deps...)
			if tt.wantErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}
		})
	}
}

func (s *DependencySuite) TestAlwaysFail() {
	err := AlwaysFail.Check()
	s.Require().Error(err)
	s.Contains(err.Error(), `dependency check for "Always fails" failed`)
}

// TestCheckCanBeCalledMultipleTimes verifies the CmdFactory fix:
// the same Dependency instance can be checked multiple times.
func (s *DependencySuite) TestCheckCanBeCalledMultipleTimes() {
	dep := Dependency{Name: "reusable", CmdFactory: cmdFactory("true")}

	// Should succeed on every call
	s.Require().NoError(dep.Check())
	s.Require().NoError(dep.Check())
	s.Require().NoError(dep.Check())
}

// TestGlobalDependenciesAreReusable verifies predefined globals work multiple times.
func (s *DependencySuite) TestGlobalDependenciesAreReusable() {
	// AlwaysFail should fail consistently on multiple calls
	s.Require().Error(AlwaysFail.Check())
	s.Require().Error(AlwaysFail.Check())
}

// =============================================================================
// StatusSuite - tests for status.go
// =============================================================================

type StatusSuite struct {
	suite.Suite
}

func TestStatusSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(StatusSuite))
}

func (s *StatusSuite) TestCheckCmd() {
	tests := []struct {
		name          string
		deps          []Dependency
		wantErr       bool
		wantCount     int
		wantInstalled []bool
	}{
		{
			name: "all installed",
			deps: []Dependency{
				{Name: "ok1", CmdFactory: cmdFactory("true")},
				{Name: "ok2", CmdFactory: cmdFactory("true")},
			},
			wantErr:       false,
			wantCount:     2,
			wantInstalled: []bool{true, true},
		},
		{
			name: "some missing",
			deps: []Dependency{
				{Name: "installed", CmdFactory: cmdFactory("true")},
				{Name: "missing", CmdFactory: cmdFactory("false")},
			},
			wantErr:       true,
			wantCount:     2,
			wantInstalled: []bool{true, false},
		},
		{
			name: "all missing",
			deps: []Dependency{
				{Name: "fail1", CmdFactory: cmdFactory("false")},
				{Name: "fail2", CmdFactory: cmdFactory("false")},
			},
			wantErr:       true,
			wantCount:     2,
			wantInstalled: []bool{false, false},
		},
		{
			name:          "empty",
			deps:          nil,
			wantErr:       false,
			wantCount:     0,
			wantInstalled: nil,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			cmd := CheckCmd(tt.deps...)
			msg := cmd()

			result, ok := msg.(CheckResult)
			s.Require().True(ok, "expected CheckResult message")

			if tt.wantErr {
				s.Require().Error(result.Err)
			} else {
				s.Require().NoError(result.Err)
			}

			s.Require().Len(result.Statuses, tt.wantCount)

			for i, wantInstalled := range tt.wantInstalled {
				s.Equal(wantInstalled, result.Statuses[i].Installed,
					"status[%d].Installed", i)
			}
		})
	}
}

// TestCheckCmdReusable verifies CheckCmd works with global deps multiple times.
func (s *StatusSuite) TestCheckCmdReusable() {
	// Call CheckCmd twice with the same global dependency
	cmd1 := CheckCmd(AlwaysFail)
	result1 := cmd1().(CheckResult)
	s.Require().Error(result1.Err)

	cmd2 := CheckCmd(AlwaysFail)
	result2 := cmd2().(CheckResult)
	s.Require().Error(result2.Err)
}

// =============================================================================
// FormatSuite - tests for format.go
// =============================================================================

type FormatSuite struct {
	suite.Suite
}

func TestFormatSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(FormatSuite))
}

func (s *FormatSuite) TestFormatStatuses() {
	tests := []struct {
		name            string
		statuses        []Status
		wantListNames   []string
		wantHelpTexts   []string
		wantNoHelpTexts []string
	}{
		{
			name: "all installed",
			statuses: []Status{
				{Name: "Git", Help: "Install Git", Installed: true},
				{Name: "Go", Help: "Install Go", Installed: true},
			},
			wantListNames:   []string{"Git", "Go"},
			wantHelpTexts:   nil,
			wantNoHelpTexts: []string{"Install Git", "Install Go"},
		},
		{
			name: "some missing",
			statuses: []Status{
				{Name: "Git", Help: "Install Git", Installed: true},
				{Name: "Docker", Help: "Install Docker", Installed: false},
			},
			wantListNames:   []string{"Git", "Docker"},
			wantHelpTexts:   []string{"Install Docker"},
			wantNoHelpTexts: []string{"Install Git"},
		},
		{
			name: "all missing",
			statuses: []Status{
				{Name: "Git", Help: "Get Git", Installed: false},
				{Name: "Go", Help: "Get Go", Installed: false},
			},
			wantListNames: []string{"Git", "Go"},
			wantHelpTexts: []string{"Get Git", "Get Go"},
		},
		{
			name:          "empty",
			statuses:      nil,
			wantListNames: nil,
			wantHelpTexts: nil,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			list, help := FormatStatuses(tt.statuses)

			for _, name := range tt.wantListNames {
				s.Contains(list, name)
			}

			for _, text := range tt.wantHelpTexts {
				s.Contains(help, text)
			}

			for _, text := range tt.wantNoHelpTexts {
				s.NotContains(help, text)
			}

			if len(tt.statuses) == 0 {
				s.Empty(list)
				s.Empty(help)
			}
		})
	}
}

func (s *FormatSuite) TestFormatMissing() {
	statuses := []Status{
		{Name: "Git", Help: "Install Git", Installed: true},
		{Name: "Docker", Help: "Install Docker", Installed: false},
	}

	output := FormatMissing(statuses)

	s.Contains(output, "Found Missing Dependencies")
	s.Contains(output, "Git")
	s.Contains(output, "Docker")
	s.Contains(output, "Install Docker")
}
