.PHONY: build install

TAG ?= latest
PREFIX ?= kern/io

build: | node_modules
	docker build -t $(PREFIX):$(TAG) .

install:
	npm install

node_modules:
	npm install
