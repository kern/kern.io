.PHONY: build run install

export AWS_PROFILE ?= kern.io

CLOUDFRONT_DIST ?= E2GNMYUUEM9F93
INVALIDATE_PATHS ?= /index.html
S3_BUCKET ?= kern.io
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
