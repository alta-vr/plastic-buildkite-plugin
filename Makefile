@PHONY: build-linux build-win deploy build-linux-on-windows build-win-on-windows deploy-on-windows
build-linux:
	cd src && GOOS=linux GOARCH=amd64 go build -o ../hooks/checkout-bin

build-win:
	cd src && GOOS=windows GOARCH=amd64 go build -o ../hooks/checkout-win.exe

deploy: build-win build-linux

build-linux-on-windows:
	cd src && set GOOS=linux&& set GOARCH=amd64&& go build -o ../hooks/checkout-bin

build-win-on-windows:
	cd src && set GOOS=windows&& set GOARCH=amd64&& go build -o ../hooks/checkout-win.exe

deploy-on-windows: build-win-on-windows build-linux-on-windows