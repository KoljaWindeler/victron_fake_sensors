build:
	go build -o bin/autarco-bridge main.go

run:
	go run main.go

compile:
	echo "Compiling for ARM OS (Venus)"
	GOOS=linux GOARCH=arm GOARM=7 go build -o bin/arm/bridge/autarco-bridge main.go
