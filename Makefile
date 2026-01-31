.PHONY: build build-debug clean run icon

# Generate icon resource (only needed if icon.ico changes)
icon:
	cd cmd && rsrc -ico icon.ico -o rsrc.syso

build:
	go build -ldflags="-s -w -H windowsgui" -o bin/shutdown-agent.exe ./cmd/

build-debug:
	go build -o bin/shutdown-agent.exe ./cmd/

clean:
	rm -f bin/shutdown-agent.exe

run: build
	./bin/shutdown-agent.exe
