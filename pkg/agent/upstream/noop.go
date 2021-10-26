package upstream


type NoopImpl struct{}

var Noop = &NoopImpl{}

func (*NoopImpl) Start() {
}

func (*NoopImpl) Stop() {
}

func (*NoopImpl) Upload(job *UploadJob) {
}

func (*NoopImpl) UploadSync(job *UploadJob) error {
	return nil
}
