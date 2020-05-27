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
install this in the same way. Download the latest release and copy.

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

For more details, see the thread on the Victron Energy community forums here:

https://community.victronenergy.com/questions/49293/alternative-to-et340-mqtt-sma-home-manager.html

# TODO

  - [ ] Setup a start/stop script and describe how to install as a system service
  - [ ] Make builds and releases automatic
  - [ ] Install and test with a real ESS / Multigrid
  - [ ] Test against fw upgrades of the Venus OS
