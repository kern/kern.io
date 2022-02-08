.PHONY: run clean build push

GCP_BUCKET ?= kern.io

run: | node_modules
	@ npm start

clean:
	@ rm -rf build

build: clean | node_modules
	@ ./node_modules/.bin/webpack -p --output-path=build

push: build
	@ gsutil cp -R 'build/*' gs://$(S3_BUCKET)

node_modules:
	@ npm install
