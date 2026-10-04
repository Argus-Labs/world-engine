package steps

type Entry struct {
	Text   string
	Status int
	Err    error
}

func NewStep(text string) Entry {
	return Entry{
		Text:   text,
		Status: INCOMPLETE,
		Err:    nil,
	}
}
