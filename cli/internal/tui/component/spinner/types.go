package spinner

// Session represents a running spinner session.
// Update changes the displayed text. Complete stops the spinner. Quit is an alias for Complete.
type Session interface {
	Update(text string)
	Complete()
	Quit()
}
