build:
	dep ensure -v
	env GOOS=linux go build -ldflags="-s -w" -o bin/KanobugCommand handlers/KanobugCommand/main.go
	env GOOS=linux go build -ldflags="-s -w" -o bin/KanobugInteractiveComponent handlers/KanobugInteractiveComponent/main.go

.PHONY: clean
clean:
	rm -rf ./bin ./vendor Gopkg.lock

.PHONY: deploy
deploy: clean build
	sls deploy --verbose
