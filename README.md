# Victron Faker
This small program emulates the ET340 Energy Meter in a Victron ESS System. It reads
values from an existing SMA Home Manager 2.0, and publishes the result on dbus as
if it were the ET340 meter.

Use this at your own risk, I have no association with Victron or SMA and am providing
this for anyone who already has these components and wants to play around with this.

I use this privately, and it works in my timezone, your results may vary. I have it running
for around 5 years. Note that the Venus GX may not have enough CPU capacity to run this.
I moved my setup is on a Raspberry Pi after discovering updates to the Venus Portal were
sometimes delayed.

# Setup

First ensure that this will work: Try out https://github.com/mitchese/sma_home_manager_printer
which will run on your Victron GX device and try to connect to the SMA meter. The above test
program does _not_ publish its result on dbus for use by victron, only prints out the result
for your verification. It should be relatively safe to test with.

If the `sma_home_manager_printer` works and shows consistent/reliable result, then you can
install this in the same way.

## Automatic Setup

[christian1980nrw](https://github.com/christian1980nrw) has created a nice and easy install script, just
run the install.sh which should do everything below. This is not immutable, so only run it once, if it fails
then follow the manual setup below

## Manual Setup

*You don't need to compile the source code* if you don't want to (see compiling below). Head over
to the [releases](https://github.com/mitchese/shm-et340/releases) and download the latest version.
then:

  * Unzip the release zipfile, which contains the "shm-et340" binary compiled for ArmV7
  * Review [how to setup root access](https://www.victronenergy.com/live/ccgx:root_access) on the device
  * Use SCP (`scp ./shm-et340 root@192.168.xxx.xxx:`) or WinSCP to copy the shm-et340 to the Venus
  * SSH in to the venus (`ssh root@192.168.xxx.xxx`) and finally run it `./shm-et340`)

While this is running, you should see correct values for a grid meter in your Venus UI:

![Venus GX UI](img/meter_sample.gif)

![Venus UI v2](img/victron_gui_v2.gif)

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
#!/bin/bash

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

# Multiple SMA meters

If you are using multiple SMA meteres (example, a Sunny Home Manager and a Energy Meter 2)
in the same network, you will need to provide the serial number of which meter you want this
to follow.

Example:
```
SMASUSYID=1234567890 ./shm-et340
```

This is the meters' serial number, which can be found in the web UI of your inverter under
Device Configuration -> Meter on Speedwire -> Serial

# License

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.

# TODO

  - [ ] Make builds and releases automatic
  - [ ] Handle a power failure of the Home manager (or other network issues preventing updates)
