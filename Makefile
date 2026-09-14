bundle:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
		-trimpath \
		-ldflags="-s -w -buildid=" \
		-o build/TimeKeeper.exe \
		main.go
