package harness

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// Outcome is the result of a single assertion.
type Outcome uint8

// Assertion outcomes.
const (
	OutcomePass Outcome = iota
	OutcomeFail
	OutcomeSkip
)

// Result is one recorded assertion.
type Result struct {
	Scenario string
	Check    string
	Detail   string
	Tick     uint64
	Outcome  Outcome
}

// Report accumulates assertion results and prints them. It is written for a
// single-goroutine tick loop, so it holds no lock.
type Report struct {
	byScenario map[string]*scenarioTally
	order      []string
	results    []Result
	notes      []string
	color      bool
	verbose    bool
}

type scenarioTally struct {
	pass, fail, skip int
}

// NewReport builds an empty report. verbose prints every passing assertion;
// otherwise only failures, skips and notes are streamed while the run proceeds.
func NewReport(verbose bool) *Report {
	return &Report{
		byScenario: map[string]*scenarioTally{},
		color:      os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb",
		verbose:    verbose,
	}
}

func (r *Report) tally(scenario string) *scenarioTally {
	t, ok := r.byScenario[scenario]
	if !ok {
		t = &scenarioTally{}
		r.byScenario[scenario] = t
		r.order = append(r.order, scenario)
	}
	return t
}

func (r *Report) paint(code, s string) string {
	if !r.color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Pass records a satisfied assertion.
func (r *Report) Pass(scenario, check string, tick uint64) {
	r.tally(scenario).pass++
	r.results = append(r.results, Result{
		Scenario: scenario, Check: check, Tick: tick, Outcome: OutcomePass,
	})
	if r.verbose {
		fmt.Printf("%s %-14s t=%-4d %s\n", r.paint("32", "PASS"), scenario, tick, check)
	}
}

// Fail records a violated assertion and prints it immediately, so a run that
// later panics still shows everything that had already gone wrong.
func (r *Report) Fail(scenario, check string, tick uint64, format string, args ...any) {
	detail := fmt.Sprintf(format, args...)
	r.tally(scenario).fail++
	r.results = append(r.results, Result{
		Scenario: scenario, Check: check, Detail: detail, Tick: tick, Outcome: OutcomeFail,
	})
	fmt.Printf("%s %-14s t=%-4d %s\n       -> %s\n",
		r.paint("31;1", "FAIL"), scenario, tick, check, detail)
}

// Skip records an assertion that could not run (a precondition was missing).
// Skips never fail the suite but are always reported, because a silently skipped
// check is indistinguishable from a passing one otherwise.
func (r *Report) Skip(scenario, check string, tick uint64, format string, args ...any) {
	detail := fmt.Sprintf(format, args...)
	r.tally(scenario).skip++
	r.results = append(r.results, Result{
		Scenario: scenario, Check: check, Detail: detail, Tick: tick, Outcome: OutcomeSkip,
	})
	fmt.Printf("%s %-14s t=%-4d %s (%s)\n", r.paint("33", "SKIP"), scenario, tick, check, detail)
}

// Note records a diagnostic observation that is neither pass nor fail. Use it for
// behaviour worth eyeballing (measured bounce heights, whether a non-bullet body
// tunnelled) where a hard threshold would only produce flaky failures.
func (r *Report) Note(scenario string, tick uint64, format string, args ...any) {
	line := fmt.Sprintf("%-14s t=%-4d %s", scenario, tick, fmt.Sprintf(format, args...))
	r.notes = append(r.notes, line)
	fmt.Printf("%s %s\n", r.paint("36", "NOTE"), line)
}

// Totals returns the run-wide pass, fail and skip counts.
func (r *Report) Totals() (int, int, int) {
	var pass, fail, skip int
	for _, t := range r.byScenario {
		pass += t.pass
		fail += t.fail
		skip += t.skip
	}
	return pass, fail, skip
}

// Failures returns every failed assertion in the order they were recorded.
func (r *Report) Failures() []Result {
	out := make([]Result, 0, 8)
	for _, res := range r.results {
		if res.Outcome == OutcomeFail {
			out = append(out, res)
		}
	}
	return out
}

// Print writes the end-of-run summary: a per-scenario table, then a repeat of
// every failure so the verdict does not require scrolling back through the log.
func (r *Report) Print() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 78))
	fmt.Println("physics2d test game — summary")
	fmt.Println(strings.Repeat("=", 78))
	fmt.Printf("%-22s %6s %6s %6s   %s\n", "SCENARIO", "PASS", "FAIL", "SKIP", "STATUS")
	fmt.Println(strings.Repeat("-", 78))

	names := append([]string(nil), r.order...)
	sort.Strings(names)
	for _, name := range names {
		t := r.byScenario[name]
		status := r.paint("32", "ok")
		if t.fail > 0 {
			status = r.paint("31;1", "FAILED")
		} else if t.pass == 0 {
			status = r.paint("33", "no checks")
		}
		fmt.Printf("%-22s %6d %6d %6d   %s\n", name, t.pass, t.fail, t.skip, status)
	}
	fmt.Println(strings.Repeat("-", 78))

	pass, fail, skip := r.Totals()
	fmt.Printf("%-22s %6d %6d %6d\n", "TOTAL", pass, fail, skip)

	if failures := r.Failures(); len(failures) > 0 {
		fmt.Println()
		fmt.Println(r.paint("31;1", fmt.Sprintf("%d FAILING CHECK(S):", len(failures))))
		for i, f := range failures {
			fmt.Printf("  %2d. [%s] t=%d %s\n      %s\n", i+1, f.Scenario, f.Tick, f.Check, f.Detail)
		}
	}

	fmt.Println()
	if fail == 0 {
		fmt.Println(r.paint("32;1", "RESULT: all checks passed"))
	} else {
		fmt.Println(r.paint("31;1", fmt.Sprintf("RESULT: %d check(s) failed", fail)))
	}
}
