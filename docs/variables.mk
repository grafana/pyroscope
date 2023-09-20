# List of projects to provide to the make-docs script.
# Format is PROJECT[:[VERSION][:[REPOSITORY][:[DIRECTORY]]]]
# The following PROJECTS value mounts content into the agent project, at the "latest" version, which is the default if not explicitly set.
# This results in the content being served at /docs/agent/latest/.
# The source of the content is the current repository which is determined by the name of the parent directory of the git root.
# This overrides the default behavior of assuming the repository directory is the same as the project name.
PROJECTS := pyroscope::$(notdir $(basename $(shell git rev-parse --show-toplevel)))

# Set the DOC_VALIDATOR_IMAGE to match the one defined in CI.
export DOC_VALIDATOR_IMAGE := $(shell sed -En 's, *image: "(grafana/doc-validator.*)",\1,p' "$(shell git rev-parse --show-toplevel)/.github/workflows/test.yml")

# Skip some doc-validator checks.
export DOC_VALIDATOR_SKIP_CHECKS := $(shell sed -En "s, *'--skip-checks=(.+)',\1,p" "$(shell git rev-parse --show-toplevel)/.github/workflows/test.yml")
