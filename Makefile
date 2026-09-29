BINARY_NAME = tukan

.PHONY: build-windows build run test release clean

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_NAME).exe .

build:
	go build -o $(BINARY_NAME) .

run:
	go run .

test:
	go test ./...

# Release zips for every Concord server platform, in dist/
release:
	go run release.go

clean:
	rm -rf $(BINARY_NAME) $(BINARY_NAME).exe dist
