package server

type mockHealthController struct{}

func (*mockHealthController) Start()                   {}
func (*mockHealthController) Stop()                    {}
func (*mockHealthController) NotificationJSON() string { return "" }
