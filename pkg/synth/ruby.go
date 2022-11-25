package synth

import (
	"fmt"
	"strings"
)

const multiplier = 1000

type rubyGenerator struct {
}

func newRubyGenerator() *rubyGenerator {
	return &rubyGenerator{}
}

func (r *rubyGenerator) newFile(name string) {

}

func (r *rubyGenerator) function(name string, nodes []*node) string {
	cases := []string{}

	if len(nodes) > 1000 {
		nodes = nodes[:1000]
	}

	for _, n := range nodes {
		delegations := []string{}
		for _, c := range n.calls {
			delegations = append(delegations, fmt.Sprintf(`%s(%s)`, c.name, c.parameter))
		}
		cases = append(cases, fmt.Sprintf(`when %s
      while i < %d; i += 1 ; end
      %s`, n.key, n.self*multiplier, strings.Join(delegations, "\n      ")))
	}

	return fmt.Sprintf(`
def %s(val)
	i = 0
  case val
  %s
  end
end
`, name, strings.Join(cases, "\n  "))
}

func (r *rubyGenerator) program(mainKey string, requires []string) string {
	newRequires := []string{}
	for _, r := range requires {
		if r != "main.rb" {
			newRequires = append(newRequires, fmt.Sprintf(`require_relative "./%s"`, r))
		}
	}

	return fmt.Sprintf(`

	require "pyroscope"
%s


Pyroscope.configure do |config|
  config.app_name = ENV["PYROSCOPE_APPLICATION_NAME"] || "synthesized-ruby-code"
  config.server_address = ENV["PYROSCOPE_SERVER_ADDRESS"] || "http://localhost:4040"
end

while true
  main(%s)
end

`, strings.Join(newRequires, "\n"), mainKey)
}
