REGISTRY ?= nas.local:6000
APP_UID ?= 1024
VERSION ?= $(shell date +"%Y%m%d%H%M")
IMAGE := $(REGISTRY)/youcast

push: latest
	docker push $(IMAGE):$<

latest: $(VERSION)
	docker tag $(IMAGE):$< $(IMAGE):$@

$(VERSION):
	docker build --build-arg APP_UID=$(APP_UID) --build-arg VERSION=$(VERSION) -t $(IMAGE):$@ .

.PHONY: push latest $(VERSION)
