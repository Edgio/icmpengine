COMMIT=$(shell /bin/git describe --always)
DATE=$(shell /bin/date -u +"%Y-%m-%d-%H:%M")

#GITLOG=$(shell /bin/git log -n1 --pretty='%h')
#VERSION=$(shell /bin/git describe --abbrev=0 --tags)

all: clean build

#	-ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"
build:
	CGO_ENABLED=0 go build -ldflags "-X main.commit=${COMMIT} -X main.date=${DATE}" -o icmpengine main.go

clean:
	rm -rf icmpengine

lo:
	./icmpengine --dest 127.0.0.1,::1
	./icmpengine --dest 127.0.0.1,::1 --count 100

los:
	./icmpengine --dest 127.0.0.1,::1 --splay

google:
	./icmpengine --dest 8.8.8.8,8.8.4.4

race:
	go run -race main.go 
