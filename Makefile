all: build test
ci: deps-tools swagger all

install:
	@cd app && go mod download

build:
	@echo "Building service"
	@rm -rf build
	@mkdir -p build
	@cd app && go mod tidy && CGO_ENABLED=0 go build -o ../build/go-sui-test ./cmd/go-sui-test
	@chmod +x ./build/go-sui-test
	@echo Build successful

clean:
	rm -rf build

run: build
	@echo "Running transactions service"
	@./build/go-sui-test

test: build
	@echo "Running unit tests"
	@cd app && go test -v ./...

test-integration:
	@echo "Running integration tests"

deps-tools:
	@cd app && go get -u github.com/swaggo/swag/cmd/swag && go install github.com/swaggo/swag/cmd/swag


swagger:
	@echo "Generating swagger documentation"
	${GOPATH}/bin/swag init -g transport/http/server.go -d app/internal,app/cmd,app/pkg/ --output app/docs

proto:
	@echo "Generating proto files"
	@buf generate
