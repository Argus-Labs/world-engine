package types

const (
	DefaultPageRequestLimit = uint32(10)
)

// IsEmptyOrDefault returns true if the page request is nil, or if it only contains default values.
func IsEmptyOrDefault(pr *PageRequest) bool {
	if pr == nil || ((pr.Key == nil || len(pr.Key) == 0) && pr.Limit == 0) {
		return true
	}
	return false
}

// DefaultPageRequest returns a default page request with a nil key, and the limit set to DefaultPageRequestLimit.
func DefaultPageRequest() *PageRequest {
	return &PageRequest{
		Key:   nil,
		Limit: DefaultPageRequestLimit,
	}
}

// ExtractPageRequest first checks if given a valid page request. If so, it returns the key and limit.
// Else, it returns the default key (nil) and limit.
func ExtractPageRequest(pr *PageRequest) ([]byte, uint32) {
	if !IsEmptyOrDefault(pr) {
		// handle the case where we were given a key, but no limit. in this case, we need to set the limit
		// to default, otherwise, it will be 0 and the query will not return anything.
		if pr.Limit == 0 {
			pr.Limit = DefaultPageRequestLimit
		}
		return pr.Key, pr.Limit
	}
	df := DefaultPageRequest()
	return df.Key, df.Limit
}
