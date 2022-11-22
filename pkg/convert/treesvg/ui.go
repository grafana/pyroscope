package treesvg

type ui struct{}

func (u *ui) ReadLine(prompt string) (string, error) {
	return "", nil
}
func (u *ui) Print(...interface{}) {
}
func (u *ui) PrintErr(...interface{}) {
}
func (u *ui) IsTerminal() bool {
	return false
}
func (u *ui) WantBrowser() bool {
	return false
}
func (u *ui) SetAutoComplete(complete func(string) string) {
}
