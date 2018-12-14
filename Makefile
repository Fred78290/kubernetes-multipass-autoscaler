all: build
VERSION_MAJOR ?= 0
VERSION_MINOR ?= 1
VERSION_BUILD ?= 0
VERSION ?= v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)
DEB_VERSION ?= $(VERSION_MAJOR).$(VERSION_MINOR)-$(VERSION_BUILD)
TAG?=v$(VERSION_MAJOR).$(VERSION_MINOR).$(VERSION_BUILD)
FLAGS=
ENVVAR=
GOOS?=linux
GOARCH?=amd64
REGISTRY?=registry.gitlab.com/frederic.boltz/k8s-autoscaler
BASEIMAGE?=k8s.gcr.io/debian-base-amd64:0.3.2

VERSION_LDFLAGS := -X github.com/Fred78290/kubernetes-multipass-autoscaler/pkg/version.version=$(VERSION)

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
	go get github.com/tools/godep

build:
	$(ENVVAR) GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="$(VERSION_LDFLAGS)" -a -o out/multipass-autoscaler-$(GOOS)-$(GOARCH) ${TAGS_FLAG}

build-binary: clean
	make -e GOOS=linux -e GOARCH=amd64 build
	make -e GOOS=darwin -e GOARCH=amd64 build

test-unit: clean deps build
	$(ENVVAR) go test --test.short -race ./... $(FLAGS) ${TAGS_FLAG}

dev-release: build-binary execute-release
	@echo "Release ${TAG}${FOR_PROVIDER} completed"

make-image:
	docker build --pull --build-arg BASEIMAGE=${BASEIMAGE} \
	    -t ${REGISTRY}/kubernetes-multipass-autoscaler${PROVIDER}:${TAG} .

push-image:
	./push_image.sh ${REGISTRY}/kubernetes-multipass-autoscaler${PROVIDER}:${TAG}

execute-release: make-image push-image

clean:
#	sudo rm -rf out

format:
	test -z "$$(find . -path ./vendor -prune -type f -o -name '*.go' -exec gofmt -s -d {} + | tee /dev/stderr)" || \
    test -z "$$(find . -path ./vendor -prune -type f -o -name '*.go' -exec gofmt -s -w {} + | tee /dev/stderr)"

docker-builder:
	docker build -t autoscaling-builder ./builder

build-in-docker: docker-builder
	docker run --rm -v `pwd`:/gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler/ autoscaling-builder:latest bash \
		-c 'cd /gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler \
		&& BUILD_TAGS=${BUILD_TAGS} make build-binary'

release: build-in-docker execute-release
	@echo "Full in-docker release ${TAG}${FOR_PROVIDER} completed"

container: clean build-in-docker make-image
	@echo "Created in-docker image ${TAG}${FOR_PROVIDER}"

test-in-docker: clean docker-builder
	docker run -v `pwd`:/gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler/ \
		autoscaling-builder:latest bash \
		-c 'cd /gopath/src/github.com/Fred78290/kubernetes-multipass-autoscaler \
		&& godep go test ./... ${TAGS_FLAG}'

.PHONY: all deps build test-unit clean format execute-release dev-release docker-builder build-in-docker release generate

