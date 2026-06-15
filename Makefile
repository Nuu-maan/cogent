BINARY := cogent
PKG := ./...

.PHONY: build test fmt vet lint run clean

build:
	go build -o $(BINARY) ./cmd/cogent

test:
	go test $(PKG)

fmt:
	gofmt -w .

vet:
	go vet $(PKG)

lint:
	golangci-lint run

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY) $(BINARY).exe
