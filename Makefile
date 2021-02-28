ALL_ARCH = amd64 arm64
all: $(addprefix build-arch-,$(ALL_ARCH))

VERSION_MAJOR ?= 1
VERSION_MINOR ?= 18
VERSION_BUILD ?= 16
DEB_VERSION ?= $(VERSION_MAJOR).$(VERSION_MINOR)-$(VERSION_BUILD)
TAG?=v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)
FLAGS=
LDFLAGS?=-s
ENVVAR=CGO_ENABLED=0 GO111MODULE=off
GOOS?=linux
GOARCH?=$(shell go env GOARCH)
REGISTRY?=fred78290
BASEIMAGE?=gcr.io/distroless/static:latest
BUILD_DATE?=`date +%Y-%m-%dT%H:%M:%SZ`
VERSION_LDFLAGS=-X main.phVersion=$(TAGS)

ifdef BUILD_TAGS
  TAGS_FLAG=--tags ${BUILD_TAGS}
  PROVIDER=-${BUILD_TAGS}
  FOR_PROVIDER=" for ${BUILD_TAGS}"
else
  TAGS_FLAG=
  PROVIDER=
  FOR_PROVIDER=
endif

deps:
	go mod vendor
#	wget "https://raw.githubusercontent.com/Fred78290/autoscaler/master/cluster-autoscaler/cloudprovider/grpc/grpc.proto" -O grpc/grpc.proto
#	protoc -I . -I vendor grpc/grpc.proto --go_out=plugins=grpc:.

build: build-arch-$(GOARCH)

build-arch-%: clean-arch-% deps
	$(ENVVAR) GOOS=$(GOOS) GOARCH=$* go build -ldflags="-X main.phVersion=$(TAG) -X main.phBuildDate=$(BUILD_DATE) $(LDFLAGS)" -a -o out/multipass-autoscaler-$* ${TAGS_FLAG}

build-binary: deps build-binary-arch-$(GOARCH)

build-binary-arch-%: clean-arch-% deps
	$(ENVVAR) GOOS=$(GOOS) GOARCH=$* go build -ldflags="-X main.phVersion=$(TAG) -X main.phBuildDate=$(BUILD_DATE)" -a -o out/multipass-autoscaler-$* ${TAGS_FLAG}

test-unit: clean deps
	./scripts/run-tests.sh'

dev-release: dev-release-arch-$(GOARCH)

dev-release-arch-%s: build-binary-arch-%s execute-release-arch-%s
	@echo "Release ${TAG}${FOR_PROVIDER} completed"

clean: clean-arch-$(GOARCH)

clean-arch-%:
	rm -f cluster-autoscaler-$*

format:
	test -z "$$(find . -path ./vendor -prune -type f -o -name '*.go' -exec gofmt -s -d {} + | tee /dev/stderr)" || \
    test -z "$$(find . -path ./vendor -prune -type f -o -name '*.go' -exec gofmt -s -w {} + | tee /dev/stderr)"

docker-builder:
	docker build -t kubernetes-multipass-autoscaler-builder ./builder

build-in-docker: build-in-docker-arch-$(GOARCH)

build-in-docker-arch-%: clean-arch-%  docker-builder
	docker run --rm -v `pwd`:/gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler/:Z kubernetes-multipass-autoscaler-builder:latest \
		bash -c 'cd /gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler && BUILD_TAGS="${BUILD_TAGS}" make -e REGISTRY=${REGISTRY} -e TAG=${TAG} -e BUILD_DATE=`date +%Y-%m-%dT%H:%M:%SZ` build-binary-arch-$*'

release: $(addprefix build-in-docker-arch-,$(ALL_ARCH)) execute-release
	@echo "Full in-docker release ${TAG}${FOR_PROVIDER} completed"

container: container-arch-$(GOARCH)

container-arch-%: build-in-docker-arch-%
	@echo "Created in-docker image ${TAG}${FOR_PROVIDER}"

test-in-docker: clean docker-builder
	docker run --rm -v `pwd`:/gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler/ kubernetes-multipass-autoscaler-builder:latest bash \
		-c 'cd /gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler && bash ./scripts/run-tests.sh'

.PHONY: all deps build test-unit clean format execute-release dev-release docker-builder build-in-docker release generate

