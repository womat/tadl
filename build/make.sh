#!/bin/bash
export GOARCH=arm
export GOOS=linux
go build -o ../bin/tadl ../cmd/tadl.go

# copy image to raspberrypi4
#scp -i ~/OneDrive/ssh-keys/pi/x  ../bin/manchester pi@breakout:/tmp
scp ../bin/tadl pv@heatpump:/tmp
#scp -i ~/OneDrive/ssh-keys/pi/x  ../config/tadl.yaml pi@heatpump:/tmp

echo '# logon on raspberrypi4'
echo '# ssh pv@breakout'

echo '# install tadl on target system'
echo '# chmod 755 /tmp/tadl ;/tmp/tadl --config /tmp/tadl.yaml'