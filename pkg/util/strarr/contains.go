package strarr

func Contains(arr []string, searchValue string) bool {
	for _, v := range arr {
		if searchValue == v {
			return true
		}
	}
	return false
}
