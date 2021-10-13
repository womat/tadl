set GOARCH=arm
set GOOS=linux
go build -o ..\bin\tadl ..\cmd\tadl.go

rem set GOARCH=386
rem set GOOS=windows
rem go build -o ..\bin\tadl.exe ..\cmd\tadl.go