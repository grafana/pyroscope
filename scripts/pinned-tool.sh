#!/bin/bash
echo "$(go list -m -f '{{.Dir}}' $1)"
