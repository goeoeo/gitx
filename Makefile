#cmd=patch
cmd=go run main.go
p=doc #项目
j=VM-1888 #jiraID
b=dev #分支 可以逗号分隔

build:
	go build -o $(shell echo $$GOPATH/bin/gitx)  main.go

.PHONY: bin
bin:
	GOOS=linux GOARCH=amd64 go build -o $(shell echo $$PWD/bin/gitx_Linux_x86_64) main.go
	GOOS=darwin GOARCH=amd64 go build -o $(shell echo $$PWD/bin/gitx_Darwin_x86_64) main.go
	GOOS=darwin GOARCH=arm64 go build -o $(shell echo $$PWD/bin/gitx_Darwin_arm64) main.go
	GOOS=windows GOARCH=amd64 go build -o $(shell echo $$PWD/bin/gitx_Windows_x86_64.exe) main.go


add:
	$(cmd) jira -a=add -p=$(p) -j=$(j) -b=$(b)

del:
	$(cmd) jira -a=del -p=$(p) -j=$(j)

print:
	$(cmd) jira -a=print

clear:
	$(cmd) jira -a=clear

push:
	$(cmd) push -p=$(p) -j=$(j)  -b=$(b)


git_clear:
	git filter-branch --force --prune-empty --index-filter 'git rm -rf --cached --ignore-unmatch bin/*' --tag-name-filter cat -- --all
