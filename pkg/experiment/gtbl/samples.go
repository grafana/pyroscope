package gtbl

type SourceInfoFrame struct {
	LineNumber   uint64 // TODO: type SourceLineno uint64
	FunctionName string
	FilePath     string
}
