package agent

type ProcessHelper interface {
	FindAllSubprocesses(pid int) []int
	PidExists(pid int32) (bool, error)
}

type noopProcessHelperImpl struct{}

func (noopProcessHelperImpl) FindAllSubprocesses(_ int) []int {
	return []int{}
}
func (noopProcessHelperImpl) PidExists(_ int32) (bool, error) {
	return true, nil
}

var NoopProcessHelper = noopProcessHelperImpl{}
