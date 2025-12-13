// Copyright 2025 Sencillo
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tpl

func Makefile() []byte {
	return []byte(`PROJECT_NAME := "{{ .Name }}"
PKG := "{{ .Module }}"
PKG_LIST := $(shell go list ${PKG}/... | grep -v /vendor/)
GO_FILES := $(shell find . -name '*.go' | grep -v /vendor/ | grep -v _test.go)
LOCAL_VERSION := $(shell if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then git describe --exact-match --tags HEAD 2>/dev/null || echo "dev-$(shell git rev-parse --short HEAD)"; else echo "dev"; fi)
GOOS=$(shell go env GOOS)
GOARCH=$(shell go env GOARCH)
GOPRIVATE=github.com/SencilloDev

.PHONY: all build docker deps clean test coverage lint docker-local edgedb k8s-up k8s-down docker-delete docs update-local deploy-local

all: build

deps: ## Get dependencies
{{"\t"}}go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

lint: deps ## Lint the files
{{"\t"}}go vet
{{"\t"}}gocyclo -over 10 -ignore "generated" ./

test: lint ## Run unittests
{{"\t"}}go test -v ./...

coverage: ## Create test coverage report
{{"\t"}}go test -cover ./...
{{"\t"}}go test ./... -coverprofile=cover.out && go tool cover -html=cover.out -o coverage.html

goreleaser: tidy ## Creates local multiarch releases with GoReleaser
{{"\t"}}goreleaser release --snapshot --rm-dist

tidy: ## Pull in dependencies
{{"\t"}}go mod tidy && go mod vendor

fmt: ## Format All files
{{"\t"}}go fmt ./...

build: ## Builds the binary on the current platform
{{"\t"}}go build -mod=vendor -a -ldflags "-w -X '$(PKG)/cmd.Version=$(VERSION)'" -o $(PROJECT_NAME)ctl

docs: ## Builds the cli documentation
{{"\t"}}mkdir -p docs
{{"\t"}}./{{ .Name }}ctl docs

schema: ## Generates boilerplate code from the graph/schema.graphqls file
{{"\t"}}go run github.com/99designs/gqlgen update

clean: ## Reset everything
{{"\t"}}git clean -fd
{{"\t"}}git clean -fx
{{"\t"}}git reset --hard

help: ## Display this help screen
{{"\t"}}@grep -h -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
`)
}

func Dockerfile() []byte {
	return []byte(`FROM golang:alpine as builder
WORKDIR /app
ENV IMAGE_TAG=dev
RUN apk update && apk upgrade && apk add --no-cache ca-certificates git
RUN update-ca-certificates
ADD . /app/
ARG VERSION
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -ldflags="-s -w -X '{{ .Module }}/cmd.Version=${VERSION}'" -installsuffix cgo -o {{ .Name }}ctl .

FROM builder AS tester
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

FROM scratch

COPY --from=builder /app/{{ .Name }}ctl .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

ENTRYPOINT ["./{{ .Name }}ctl"]
`)
}

func GoReleaser() []byte {
	return []byte(`version: 2
project_name: [% .Name %]ctl

builds:
  - env:
      - CGO_ENABLED=0
      - IMAGE_TAG={{.Tag}}
      - "GO111MODULE=on"
    goos:
      - linux
      - windows
      - darwin
    goarch:
      - amd64
      - arm64
    ldflags: "-extldflags= -w -X 'github.com/SencilloDev/[% .Name %]/cmd.Version={{.Tag}}'"
    flags:
      - -mod=vendor

archives:
  - formats: [binary]
    name_template: >-
      {{ .ProjectName }}_
      {{- .Os }}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else if eq .Arch "386" }}i386
      {{- else }}{{ .Arch }}{{ end }}
      {{- if .Arm }}v{{ .Arm }}{{ end }}
    # use zip for windows archives
    format_overrides:
      - goos: windows
        formats: [binary]


changelog:
  sort: asc
  filters:
    exclude:
      - "^docs:"
      - "^test:"
source:
  enabled: true

release:
  footer: >-

    ---

    Released by [GoReleaser](https://github.com/goreleaser/goreleaser).

`)
}

func TestWorkflow() []byte {
	return []byte(`name: test
on: 
  push:
    paths:
      - '**.go'
  workflow_call:
jobs:
  test:
    strategy:
      matrix:
        go-version: [ 1.22.x ]
        os: [ ubuntu-latest ]
    runs-on: ${{ matrix.os }}
    steps:
      - name: Install Go
        uses: actions/setup-go@v2
        with:
          go-version: ${{ matrix.go-version }}
      - name: Checkout code
        uses: actions/checkout@v2
      - name: Test
        run: make test
      - name: Coverage
        run: make coverage
      - name: store coverage
        uses: actions/upload-artifact@v4
        with:
          name: test-coverage
          path: ./coverage.html 
`)
}

func ReleaseWorkflow() []byte {
	return []byte(`name: deploy dev
on:
  push:
    branches:
      - main
permissions:
  id-token: write
  contents: read
jobs:
  test:
    uses: ./.github/workflows/test.yaml
  release:
    permissions:
      id-token: write
      contents: write
    runs-on: ubuntu-latest
    needs: [test]
    steps:
      - name: Checkout code
        uses: actions/checkout@v2
      - name: fly deploy
        uses: superfly/flyctl-actions/setup-flyctl@master
      - run: flyctl deploy --config fly.toml
        env:
          FLY_API_TOKEN: ${{ secrets.FLY_DEV_API_TOKEN }}
`)
}

func Gitignore() []byte {
	return []byte(`{{ .Name }}ctl
sgoctl*
dist/
output/
`)
}
