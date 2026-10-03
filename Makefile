PROJECT_NAME := kvmrun-dashboard
PROJECT_REPO := github.com/kvmrun/dashboard

GOLANG_IMAGE := golang:1.25-trixie
DEVTOOLS_IMAGE := 0xef53/debian-dev:bookworm

CWD := $(shell pwd)

ifeq (,$(wildcard /etc/debian_version))
    SYSTEMD_UNITDIR ?= /usr/lib/systemd/system
else
    SYSTEMD_UNITDIR ?= /lib/systemd/system
endif

DOCKER_BUILD_ARGS := \
    -w /go/$(PROJECT_NAME) \
    -v $(PROJECT_NAME)_pkg:/go/pkg \
    -v $(CWD):/go/$(PROJECT_NAME) \
    -v $(CWD)/scripts/build.sh:/usr/local/bin/build.sh \
    -e GOBIN=/go/$(PROJECT_NAME)/bin \
    --entrypoint build.sh

DOCKER_TESTS_ARGS := \
    -w /go/$(PROJECT_NAME) \
    -v $(PROJECT_NAME)_pkg:/go/pkg \
    -v $(CWD):/go/$(PROJECT_NAME)

DOCKER_DEB_ARGS := \
    -w /root/source \
    -v $(CWD):/root/source:ro \
    -v $(CWD)/packages:/root/source/packages \
    -v $(CWD)/scripts/build-deb.sh:/usr/local/bin/build-deb.sh \
    -e PROJECT_NAME=$(PROJECT_NAME) \
    --entrypoint build-deb.sh

binaries = \
    bin/dashboard

.PHONY: all build clean

all: build

$(binaries):
	@echo "##########################"
	@echo "#  Building binaries     #"
	@echo "##########################"
	@echo
	install -d bin
	docker run --rm -i $(DOCKER_BUILD_ARGS) $(GOLANG_IMAGE)
	@echo
	@echo "==================="
	@echo "Successfully built:"
	ls -lh bin/
	@echo

build: $(binaries)

tests:
	@echo "##########################"
	@echo "#  Running tests         #"
	@echo "##########################"
	@echo
	docker run --rm -i $(DOCKER_TESTS_ARGS) $(GOLANG_IMAGE) go test ./...
	@echo
	@echo

install: $(binaries)
	install -d $(DESTDIR)/usr/lib/$(PROJECT_NAME)
	cp -t $(DESTDIR)/usr/lib/$(PROJECT_NAME) $(binaries)
	install -d $(DESTDIR)$(SYSTEMD_UNITDIR)
	cp -t $(DESTDIR)$(SYSTEMD_UNITDIR) contrib/kvmrun-dashboard.service
	@echo

deb-package: $(binaries)
	@echo "##########################"
	@echo "#  Building deb package  #"
	@echo "##########################"
	@echo
	install -d packages
	docker run --rm -i $(DOCKER_DEB_ARGS) $(DEVTOOLS_IMAGE)
	@echo
	@echo "==================="
	@echo "Successfully built:"
	@find packages -type f -name '*.deb' -printf "%p\n"
	@echo

clean:
	rm -Rvf bin packages vendor

