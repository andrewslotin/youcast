REGISTRY ?= ghcr.io/andrewslotin
APP_UID ?= 1024
VERSION ?= $(shell date +"%Y%m%d%H%M")
IMAGE := $(REGISTRY)/youcast

BUILDC ?= $(shell command -v podman 2>/dev/null)

ifeq ($(BUILDC),)
BUILDC = $(shell command -v docker 2>/dev/null)
endif

ifeq ($(BUILDC),)
$(error "No build tool found. Please install podman or docker")
endif

push: latest
	$(BUILDC) push $(IMAGE):$<

latest: $(VERSION)
	$(BUILDC) tag $(IMAGE):$< $(IMAGE):$@

$(VERSION):
	$(BUILDC) build --build-arg APP_UID=$(APP_UID) --build-arg VERSION=$(VERSION) -t $(IMAGE):$@ .

.PHONY: push latest $(VERSION)
