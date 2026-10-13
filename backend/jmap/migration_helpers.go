package jmap

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
