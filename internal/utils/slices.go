package utils

func Filter[T any](slice []T, f func(T) bool) []T {
	var n []T
	for _, e := range slice {
		if f(e) {
			n = append(n, e)
		}
	}
	return n
}

func First[T any](slice []T, f func(T) bool) *T {
	for _, e := range slice {
		if f(e) {
			return &e
		}
	}
	return nil
}
