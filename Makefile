REPO_ROOT           := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
VERSION             := $(shell cat $(REPO_ROOT)/VERSION)
EFFECTIVE_VERSION   := $(VERSION)-$(shell git rev-parse HEAD | cut -c1-8)
FROM_IMAGE_BUILDER  := docker.io/library/golang:1.16
FROM_IMAGE          := registry.access.redhat.com/ubi8/ubi-minimal:8.5
IMAGE_NAME          := myregistry/yumsecupdater
IMAGE               := $(IMAGE_NAME):$(EFFECTIVE_VERSION)
IMAGE_LATEST        := $(IMAGE_NAME):latest

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-extldflags "-static"'

.PHONY: print-image-tag
print-image-tag:
	@echo $(IMAGE)

.PHONY: print-image-tag-latest
print-image-tag-latest:
	@echo $(IMAGE_LATEST)

.PHONY: build-image
build-image:
	buildah bud \
        --build-arg FROM_IMAGE_BUILDER=$(FROM_IMAGE_BUILDER) \
        --build-arg FROM_IMAGE=$(FROM_IMAGE) \
        -t $(IMAGE) \
        -t $(IMAGE_LATEST) .

.PHONY: push-image
push-image:
	buildah push $(IMAGE)
	buildah push $(IMAGE_LATEST)

.PHONY: kaniko
kaniko:
	/kaniko/executor \
        --context $$CI_PROJECT_DIR \
        --dockerfile $$CI_PROJECT_DIR/Containerfile \
        --build-arg FROM_IMAGE_BUILDER=$(FROM_IMAGE_BUILDER) \
        --build-arg FROM_IMAGE=$(FROM_IMAGE) \
        --destination $(IMAGE) \
        --destination $(IMAGE_LATEST)

.PHONY: build-push-image
build-push-image: build-image push-image
