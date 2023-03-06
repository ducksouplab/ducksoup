dev:
	@go build && DUCKSOUP_MODE=DEV GST_DEBUG=2,videodecoder:1 ./ducksoup
frontbuild:
	@go build && DUCKSOUP_MODE=FRONT_BUILD ./ducksoup
deps:
	@go get -t -u ./... && go mod tidy
test:
	@DUCKSOUP_TEST_ROOT=`pwd`/ go test ./...
testv:
	@DUCKSOUP_TEST_ROOT=`pwd`/ go test -v ./...
dockerbuild:
	@docker build -f docker/Dockerfile.build -t ducksoup:latest .
dockerpush:
	@docker tag ducksoup ducksouplab/ducksoup && docker push ducksouplab/ducksoup:latest
