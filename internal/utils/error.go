package utils

type ReasonError struct {
	Reason  string
	Message string
}

func (e ReasonError) Error() string {
	return e.Message
}
