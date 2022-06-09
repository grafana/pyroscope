package slices

func StringContains(arr []string, searchValue string) bool {
	for _, v := range arr {
		if searchValue == v {
			return true
		}
	}
	return false
}

func IntContains(arr []int, searchValue int) bool {
	for _, v := range arr {
		if searchValue == v {
			return true
		}
	}
	return false
}
