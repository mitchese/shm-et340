#!/bin/sh
mkdir /data/drivers
mkdir /data/drivers/shm-et340
mkdir /data/drivers/shm-et340/service
cd /data/drivers/shm-et340
wget https://github.com/mitchese/shm-et340/releases/download/v0.3/shm-et340
chmod +x ./shm-et340
cd /data/drivers/shm-et340/service
wget https://raw.githubusercontent.com/christian1980nrw/shm-et340-fixed-vrm-portal/master/service/run
chmod +x ./run
ln -s  /data/drivers/shm-et340/service /service/shm-et340
echo >> /data/rc.local
echo "ln -s /data/drivers/shm-et340/service /service/shm-et340" >> /data/rc.local
chmod +x /data/rc.local
echo Installation finished. Please edit the file /data/drivers/shm-et340/service/run and change the IP adress of your smartmeter.
echo Please ensure in your routers DHCP-service that the smartmeter is always getting the same IP adress.
echo Note: This installation will survive a firmware update.
