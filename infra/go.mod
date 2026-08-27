// This directory is a TypeScript AWS CDK project, not Go. The go.mod marks it as
// a separate module boundary so `go build/vet/test ./...` in the root module
// skips it (and its node_modules, which contains invalid Go template files).
module questrade-ynab-infra

go 1.25
