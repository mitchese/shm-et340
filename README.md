# Victron Faker
This small program emulates the ET340 Energy Meter in a Victron ESS System. It reads
values from an existing SMA Home Manager 2.0, and publishes the result on dbus as
if it were the ET340 meter.

Use this at your own risk, I have no association with Victron or SMA and am providing
this for anyone who already has these components and wants to play around with this.

I use this privately, and it works in my timezone, your results may vary

# Setup

First ensure that this will work: Try out https://github.com/mitchese/sma_home_manager_printer
which will run on your Victron GX device and try to connect to the SMA meter. The above test
program does _not_ publish its result on dbus for use by victron, only prints out the result
for your verification. It should be relatively safe to test with.

If the `sma_home_manager_printer` works and shows consistent/reliable result, then you can
install this in the same way.

*You don't need to compile the source code* if you don't want to (see compiling below). Head over
to the [releases](https://github.com/mitchese/shm-et340/releases) and download the latest version.
then:

  * Unzip the release zipfile, which contains the "shm-et340" binary compiled for ArmV7
  * Review [how to setup root access](https://www.victronenergy.com/live/ccgx:root_access) on the device
  * Use SCP (`scp ./shm-et340 root@192.168.xxx.xxx:`) or WinSCP to copy the shm-et340 to the Venus
  * SSH in to the venus (`ssh root@192.168.xxx.xxx`) and finally run it `./shm-et340`)

While this is running, you should see correct values for a grid meter in your Venus UI:

![Venus GX UI](img/meter_sample.gif)

On the console of your GX device, you should see regular updates, around once per second:

```
root@victronvenusgx:~# ./shm-et340
INFO[0000] Successfully connected to dbus and registered as a meter... Commencing reading of the SMA meter
INFO[0000] Meter update received: 6677.15 kWh bought and 3200.45 kWh sold, 681.3 W currently flowing
INFO[0001] Meter update received: 6677.15 kWh bought and 3200.45 kWh sold, 694.1 W currently flowing
INFO[0002] Meter update received: 6677.15 kWh bought and 3200.45 kWh sold, 686.3 W currently flowing
```

If this does not work, try to `export LOG_LEVEL="debug"` first, which should print out significantly more
information on what's happening.

# Starting at boot

The above steps will start it once, which will run until the next reboot. Doing the following will start it on every boot

Thanks to [ricott](https://github.com/ricott) for the tip here. The full description of how to start on boot
can be found [here](https://www.victronenergy.com/live/ccgx:root_access). Basically, add the call to `/data/rc.local`.

Mine tried to start before the network was up, which resulted in an error and it not starting. To 'fix' this, I just wait 15s in the
rc.local before trying to start the script ... not great but it works.

```
root@beaglebone:~# cat /data/rc.local
!#/bin/bash

sleep 15
setsid /data/home/root/shm-et340 > /dev/null 2>/dev/null &

root@beaglebone:~# ls -l /data/rc.local
-rwxr-xr-x    1 root     root            81 Feb 18 15:38 /data/rc.local

```

# Compiling from source

For windows, and more detailed instructions, head on over to [Schnema1's fork](https://github.com/Schnema1/sma_home_manager_printer)

To compile this for the Venus GX (an Arm 7 processor), you can easily cross-compile with the following:

`GOOS=linux GOARCH=arm GOARM=7 go build`


# Additional Info

For more details, see the thread on the Victron Energy community forums here:

https://community.victronenergy.com/questions/49293/alternative-to-et340-mqtt-sma-home-manager.html

# TODO

  - [ ] Setup a start/stop script and describe how to install as a system service
  - [ ] Make builds and releases automatic
  - [ ] Test against fw upgrades of the Venus OS
  - [ ] Handle a power failure of the Home manager (or other network issues preventing updates)
