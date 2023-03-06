dev:
	@go build && DUCKSOUP_MODE=DEV GST_DEBUG=2,videodecoder:1 ./ducksoup
front:
	@go build && DUCKSOUP_MODE=BUILD_FRONT ./ducksoup
deps:
	@go get -t -u ./... && go mod tidy
test:
	@DUCKSOUP_TEST_ROOT=`pwd`/ go test ./...
testv:
	@MDUCKSOUP_TEST_ROOT=`pwd`/ go test -v ./...