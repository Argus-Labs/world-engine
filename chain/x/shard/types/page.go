package types

const (
	DefaultPageRequestLimit = uint32(10)
)

func IsEmptyOrDefault(pr *PageRequest) bool {
	if pr == nil || ((pr.Key == nil || len(pr.Key) == 0) && pr.Limit == 0) {
		return true
	}

	return false
}

func DefaultPageRequest() *PageRequest {
	return &PageRequest{
		Key:   nil,
		Limit: DefaultPageRequestLimit,
	}
}

func ExtractPageRequest(pr *PageRequest) (key []byte, limit uint32) {
	if !IsEmptyOrDefault(pr) {
		if pr.Limit == 0 {
			pr.Limit = DefaultPageRequestLimit
		}
		return pr.Key, pr.Limit
	}

	df := DefaultPageRequest()
	return df.Key, df.Limit
}
