#!/bin/sh
mkdir /data/drivers/shm-et340
mkdir /data/drivers/shm-et340/service
cd /data/drivers/shm-et340
wget https://github.com/christian1980nrw/shm-et340-fixed-vrm-portal/raw/master/shm-et340
chmod +x ./shm-et340
cd /data/drivers/shm-et340/service
wget https://raw.githubusercontent.com/christian1980nrw/shm-et340-fixed-vrm-portal/master/shm-et340-service.sh
chmod +x ./shm-et340-service.sh
mv ./shm-et340-service.sh ./run
ln -s  /data/drivers/shm-et340/service /service/shm-et340
echo >> /data/rc.local
echo "ln -s /data/drivers/shm-et340/service /service/shm-et340" >> /data/rc.local
echo Installation finished. Please edit the file /data/drivers/shm-et340/service/run and change the IP adress of your smartmeter.
echo Please ensure (in your routers DHCP-service) that it is always getting the same IP adress.
