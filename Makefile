DOCKER_NAME=rootsdev/trellis-cli-dev
DOCKER_RUN=docker run --rm -it -v $(shell pwd):/app -v $(GOPATH):/go
RUN=$(DOCKER_RUN) $(DOCKER_NAME)

.PHONY: docker docker-no-cache
docker:
	docker build -t $(DOCKER_NAME) .
docker-no-cache:
	docker build -t $(DOCKER_NAME) --no-cache .

.PHONY: shell
shell:
	$(RUN) bash

.PHONY: test
test:
	$(RUN) sh -c 'go build -v -o $$TEST_BINARY && go test -v ./...'
