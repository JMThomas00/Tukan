BINARY_NAME = tukan
CMD_PATH    = ./cmd/tukan

.PHONY: build-windows build run clean

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o $(BINARY_NAME).exe $(CMD_PATH)

build:
	go build -o $(BINARY_NAME) $(CMD_PATH)

run:
	go run $(CMD_PATH)

clean:
	rm -f $(BINARY_NAME) $(BINARY_NAME).exe
