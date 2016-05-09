.PHONY: build run install

TAG ?= latest
PREFIX ?= kern/io

build: | node_modules
	docker build -t $(PREFIX):$(TAG) .

run: | node_modules
	@ node index.js

install:
	@ npm install

node_modules:
	@ npm install
