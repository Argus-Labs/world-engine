package debugger

type Cmd struct {
	Pause  *PauseCmd  `cmd:"" group:"Debugger Commands:" help:"Pause the execution of ticks in your Cardinal game environment"`
	Resume *ResumeCmd `cmd:"" group:"Debugger Commands:" help:"Resume the execution of ticks after a pause"`
	Step   *StepCmd   `cmd:"" group:"Debugger Commands:" help:"Execute a single tick (only works when paused)"`
	Reset  *ResetCmd  `cmd:"" group:"Debugger Commands:" help:"Reset the world to its initial state (before tick 0)"`
}
