package synth

import (
	"fmt"
	"strings"
)

const multiplier = 1000

type rubyGenerator struct {
	functions []string
}

func newRubyGenerator() *rubyGenerator {
	return &rubyGenerator{}
}

func (r *rubyGenerator) function(name string, nodes []*node) {
	cases := []string{}

	for _, n := range nodes {
		delegations := []string{}
		for _, c := range n.calls {
			delegations = append(delegations, fmt.Sprintf(`%s("%s")`, c.name, c.parameter))
		}
		cases = append(cases, fmt.Sprintf(`when "%s"
      i = 0
      while i < %d; i +=1 ; end

      %s`, n.key, n.self*multiplier, strings.Join(delegations, "\n      ")))

		r.functions = append(r.functions, fmt.Sprintf(`
def %s(val)
  case val
  %s
  end
end
`, name, strings.Join(cases, "\n  ")))
	}
}

func (r *rubyGenerator) program(mainKey string) string {
	return fmt.Sprintf(`

require "pyroscope"

Pyroscope.configure do |config|
  config.app_name = ENV["PYROSCOPE_APPLICATION_NAME"] || "synthesized-ruby-code"
  config.server_address = ENV["PYROSCOPE_SERVER_ADDRESS"] || "http://localhost:4040"
end

%s
while true
  main("%s")
end

`, strings.Join(r.functions, "\n"), mainKey)
}
