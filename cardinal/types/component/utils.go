package component

// ConvertComponentMetadatasToComponents Cast an array of ComponentMetadata into an array of Component
func ConvertComponentMetadatasToComponents(comps []ComponentMetadata) []Component {
	ret := make([]Component, len(comps))
	for i, comp := range comps {
		ret[i] = comp
	}
	return ret
}
