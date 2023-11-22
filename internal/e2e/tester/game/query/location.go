package query

type LocationRequest struct {
	ID string
}

type LocationReply struct {
	X, Y int64
}
