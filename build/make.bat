set GOARCH=arm
set GOOS=linux
go build -o ..\bin\tadl ..\cmd\tadl.go

set GOARCH=386
set GOOS=windows
go build -o ..\bin\tadl.exe ..\cmd\tadl.go