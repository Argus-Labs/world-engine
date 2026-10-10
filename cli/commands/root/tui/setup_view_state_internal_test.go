package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/argus-labs/world-engine/cli/internal/tui/kit/steps"
	"github.com/argus-labs/world-engine/cli/pkg/worldscaffold"
)

// driveToStep advances the wizard's steps.Model to the given step index by
// feeding steps.CompleteStepMsg through Update, bypassing the name input
// handler. This mirrors the arg path, where NewWorldSetupModel.Focus()es the
// input and Init drives name completion via CompleteStepCmd — so the input
// stays focused and is never blurred as the wizard advances.
func driveToStep(m WorldSetupModel, targetIndex int) WorldSetupModel {
	for m.steps.CurrentIndex() < targetIndex {
		next, _ := m.Update(steps.CompleteStepMsg{})
		m = next.(WorldSetupModel)
	}
	return m
}

// execCmd runs a tea.Cmd once and returns the produced message (nil if cmd nil).
// Only used for single-message cmds (not tea.Sequence); Sequence cmds from
// steps.Update are deliberately not executed so the unit tests never trigger
// real clone/tidy side effects.
func execCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// ---------------------------------------------------------------------------
// View() gating: the name prompt must render only while on the stepName step.
// ---------------------------------------------------------------------------

func TestViewOmitsNamePromptDuringStepClone(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	m.showTemplateList = false
	tmpl := worldscaffold.GetAvailableTemplates()[0]
	m.selectedTemplate = &tmpl
	m = driveToStep(m, 2) // stepName ✓, stepTemplate ✓, now on stepClone (index 2)

	if m.steps.CurrentIndex() != int(stepClone) {
		t.Fatalf("precondition: expected index %d (stepClone), got %d", int(stepClone), m.steps.CurrentIndex())
	}
	if m.steps.Steps[0].Status != steps.COMPLETE || m.steps.Steps[1].Status != steps.COMPLETE {
		t.Fatalf("precondition: stepName/stepTemplate should be COMPLETE")
	}
	if m.steps.Steps[2].Status != steps.INCOMPLETE {
		t.Fatalf("precondition: stepClone should be INCOMPLETE")
	}

	got := m.View()
	t.Logf("View() during stepClone:\n%s", got)

	if strings.Contains(got, "What is your game shard name?") {
		t.Errorf("View() during stepClone must NOT render the stale name prompt; got:\n%s", got)
	}
	if !strings.Contains(got, "Initialize game shard with selected template") {
		t.Errorf("View() during stepClone should still render the step list; got:\n%s", got)
	}
}

func TestViewOmitsNamePromptDuringStepTidy(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	m.showTemplateList = false
	tmpl := worldscaffold.GetAvailableTemplates()[0]
	m.selectedTemplate = &tmpl
	m = driveToStep(m, 3) // now on stepTidy (index 3)

	if m.steps.CurrentIndex() != int(stepTidy) {
		t.Fatalf("precondition: expected index %d (stepTidy), got %d", int(stepTidy), m.steps.CurrentIndex())
	}

	got := m.View()
	t.Logf("View() during stepTidy:\n%s", got)

	if strings.Contains(got, "What is your game shard name?") {
		t.Errorf("View() during stepTidy must NOT render the stale name prompt; got:\n%s", got)
	}
	if !strings.Contains(got, "Tidy template go.mod dependencies") {
		t.Errorf("View() during stepTidy should still render the step list; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Arg-path state corruption: Enter during clone/tidy must be ignored even
// though the name input remains focused.
// ---------------------------------------------------------------------------

func TestEnterDuringCloneIsIgnored(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	m.showTemplateList = false
	tmpl := worldscaffold.GetAvailableTemplates()[0]
	m.selectedTemplate = &tmpl
	m = driveToStep(m, 2) // on stepClone (index 2); input still focused (arg path)

	if !m.projectNameInput.Focused() {
		t.Fatalf("precondition: arg-path input should still be focused during stepClone")
	}

	// Simulate the user pressing Enter while the clone spinner animates.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := next.(WorldSetupModel)

	if cmd != nil {
		t.Errorf("Enter during stepClone should be swallowed (nil cmd); got non-nil cmd producing %T", execCmd(cmd))
	}
	if mm.steps.Steps[int(stepClone)].Status == steps.COMPLETE {
		t.Errorf(
			"stepClone must NOT be prematurely marked COMPLETE by Enter during clone; status=%d",
			mm.steps.Steps[int(stepClone)].Status,
		)
	}
	if mm.steps.CurrentIndex() != int(stepClone) {
		t.Errorf("wizard must NOT advance past stepClone on Enter; index=%d", mm.steps.CurrentIndex())
	}
	// No false "Successfully created a starter game shard" log may be appended.
	for _, l := range mm.logs {
		if strings.Contains(l, "Successfully created a starter game shard") {
			t.Errorf("false success log must not be emitted during clone; logs=%v", mm.logs)
		}
	}
}

func TestEnterDuringTidyIsIgnored(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	m.showTemplateList = false
	tmpl := worldscaffold.GetAvailableTemplates()[0]
	m.selectedTemplate = &tmpl
	m = driveToStep(m, 3) // on stepTidy (index 3); input still focused (arg path)

	if !m.projectNameInput.Focused() {
		t.Fatalf("precondition: arg-path input should still be focused during stepTidy")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := next.(WorldSetupModel)

	if cmd != nil {
		t.Errorf("Enter during stepTidy should be swallowed (nil cmd); got non-nil cmd producing %T", execCmd(cmd))
	}
	if mm.steps.Steps[int(stepTidy)].Status == steps.COMPLETE {
		t.Errorf(
			"stepTidy must NOT be prematurely marked COMPLETE by Enter during tidy; status=%d",
			mm.steps.Steps[int(stepTidy)].Status,
		)
	}
	if mm.steps.CurrentIndex() != int(stepTidy) {
		t.Errorf("wizard must NOT advance past stepTidy on Enter; index=%d", mm.steps.CurrentIndex())
	}
}

// ---------------------------------------------------------------------------
// Regression: the happy path still advances the wizard correctly.
// ---------------------------------------------------------------------------

func TestEnterOnNameStepCompletesNameStep(t *testing.T) {
	m := NewWorldSetupModel("", "DEV", "")
	m.projectNameInput.SetValue("my-game") // simulate the user typing a canonical name

	if m.steps.CurrentIndex() != int(stepName) {
		t.Fatalf("precondition: expected to start on stepName; got index %d", m.steps.CurrentIndex())
	}
	if !m.projectNameInput.Focused() {
		t.Fatalf("precondition: name input should be focused on the no-args path")
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := next.(WorldSetupModel)

	if cmd == nil {
		t.Fatalf("Enter on stepName should produce a CompleteStepCmd, got nil")
	}
	if mm.projectNameInput.Focused() {
		t.Errorf("name input should be blurred after completing the name step")
	}
	if mm.nameErr != "" {
		t.Errorf("canonical name should not set a name error; got %q", mm.nameErr)
	}

	// Drive the returned CompleteStepCmd through steps to confirm stepName is
	// marked COMPLETE and the wizard advances to stepTemplate.
	msg := execCmd(cmd)
	if _, ok := msg.(steps.CompleteStepMsg); !ok {
		t.Fatalf("cmd should produce steps.CompleteStepMsg; got %T", msg)
	}
	done, _ := mm.Update(msg)
	dd := done.(WorldSetupModel)
	if dd.steps.Steps[int(stepName)].Status != steps.COMPLETE {
		t.Errorf("stepName should be COMPLETE after Enter; got %d", dd.steps.Steps[int(stepName)].Status)
	}
	if dd.steps.CurrentIndex() != int(stepTemplate) {
		t.Errorf("wizard should advance to stepTemplate (index 1); got %d", dd.steps.CurrentIndex())
	}
}

func TestCloneFinishedCompletesStepCloneNotStepTidy(t *testing.T) {
	m := NewWorldSetupModel("my-game", "DEV", "")
	m.showTemplateList = false
	tmpl := worldscaffold.GetAvailableTemplates()[0]
	m.selectedTemplate = &tmpl
	m = driveToStep(m, 2) // on stepClone (index 2)

	// Feed the real CloneFinishedMsg (clone succeeded). handleCloneFinished
	// returns a CompleteStepCmd that must complete stepClone (index 2), not
	// stepTidy. In the buggy version an earlier stray Enter would have
	// advanced the wizard to stepTidy, so this msg would mark stepTidy done.
	next, cmd := m.Update(CloneFinishedMsg{Err: nil})
	m1 := next.(WorldSetupModel)

	msg := execCmd(cmd)
	if _, ok := msg.(steps.CompleteStepMsg); !ok {
		t.Fatalf("CloneFinishedMsg should produce steps.CompleteStepMsg; got %T", msg)
	}
	done, _ := m1.Update(msg)
	m2 := done.(WorldSetupModel)

	if m2.steps.Steps[int(stepClone)].Status != steps.COMPLETE {
		t.Errorf("stepClone should be COMPLETE after CloneFinishedMsg; got %d", m2.steps.Steps[int(stepClone)].Status)
	}
	if m2.steps.CurrentIndex() != int(stepTidy) {
		t.Errorf("wizard should advance to stepTidy (index 3); got %d", m2.steps.CurrentIndex())
	}
	if m2.steps.Steps[int(stepTidy)].Status != steps.INCOMPLETE {
		t.Errorf(
			"stepTidy should still be INCOMPLETE (real tidy not run yet); got %d",
			m2.steps.Steps[int(stepTidy)].Status,
		)
	}
}
