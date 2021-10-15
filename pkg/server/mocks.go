package server

type mockHealthController struct{}

func (mockHealthController) Notification() []string { return nil }
