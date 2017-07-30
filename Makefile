.PHONY: run clean build push

export AWS_PROFILE ?= kern.io

CLOUDFRONT_DIST ?= E2GNMYUUEM9F93
S3_BUCKET ?= kern.io
INVALIDATE_PATHS ?= / /index.html
PREFIX ?= kern/io

run: | node_modules
	@ nps start

clean:
	@ rm -rf build

build: clean | node_modules
	@ ./node_modules/.bin/webpack -p --output-path=build

push: build
	@ aws s3 cp --recursive \
			build s3://$(S3_BUCKET)
	@ aws configure set preview.cloudfront true
	@ aws cloudfront create-invalidation \
			--distribution-id $(CLOUDFRONT_DIST) \
			--paths $(INVALIDATE_PATHS)

node_modules:
	@ npm install
