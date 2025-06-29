#!/bin/sh
mkdir -p /data/drivers/shm-et340/service
cd /data/drivers/shm-et340
wget https://github.com/mitchese/shm-et340/releases/download/v0.6/shm-et340
chmod +x ./shm-et340
cd /data/drivers/shm-et340/service
wget https://raw.githubusercontent.com/mitchese/shm-et340/master/service/run
chmod +x ./run
ln -s  /data/drivers/shm-et340/service /service/shm-et340
echo >> /data/rc.local
echo "ln -s /data/drivers/shm-et340/service /service/shm-et340" >> /data/rc.local
chmod +x /data/rc.local
echo Installation finished. Please edit the file /data/drivers/shm-et340/service/run and change the IP address of your smartmeter.
echo Please ensure in your routers DHCP-service that the smartmeter is always getting the same IP address.
echo Note: This installation will survive a firmware update.
