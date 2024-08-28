package demangle

import "github.com/ianlancetaylor/demangle"

var DemangleUnspecified []demangle.Option = nil
var DemangleNoneSpecified []demangle.Option = make([]demangle.Option, 0)
var DemangleSimplified = []demangle.Option{demangle.NoParams, demangle.NoEnclosingParams, demangle.NoTemplateParams}
var DemangleTemplates = []demangle.Option{demangle.NoParams, demangle.NoEnclosingParams}
var DemangleFull = []demangle.Option{demangle.NoClones}

func ConvertDemangleOptions(o string) []demangle.Option {
	switch o {
	case "none":
		return DemangleNoneSpecified
	case "simplified":
		return DemangleSimplified
	case "templates":
		return DemangleTemplates
	case "full":
		return DemangleFull
	default:
		return DemangleUnspecified
	}
}
