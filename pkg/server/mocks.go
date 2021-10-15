package server

type mockNotifier struct{}

func (mockNotifier) NotificationText() string { return "" }
