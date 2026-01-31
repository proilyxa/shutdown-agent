.PHONY: build build-debug clean run icon

icon:
	cd cmd && rsrc -ico icon.ico -o rsrc.syso

build: icon
	go build -ldflags="-s -w -H windowsgui" -o bin/shutdown-agent.exe ./cmd/

build-debug: icon
	go build -o bin/shutdown-agent.exe ./cmd/

clean:
	rm -f bin/shutdown-agent.exe cmd/rsrc.syso

run: build
	./bin/shutdown-agent.exe
