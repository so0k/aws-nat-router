SOURCEDIR= .
SOURCES := $(shell find $(SOURCEDIR) -name '*.go')

bin/aws-nat-router: $(SOURCES)
	go build -o bin/aws-nat-router cmd/aws-nat-router/main.go