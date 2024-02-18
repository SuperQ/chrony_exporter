## main / unreleased

## 0.9.0 / 2024-02-18

* [FEATURE] Add serverstats metrics collector #62

## 0.8.0 / 2023-12-30

* [ENHANCEMENT] Improve ability to use collector as a library #55

## 0.7.0 / 2023-12-25

* [FEATURE] Add option to disable Reverse DNS Lookups #48
* [ENHANCEMENT] Add gauges for frequency, skew and update interval #51
* [BUGFIX] Fix swapped HELP text #47

## 0.6.1 / 2023-07-16

* [ENHANCEMENT] Add a command line option to force chmod on the receiving unix socket. #45

## 0.6.0 / 2023-07-16

* [FEATURE] Add support for connecting to chrony using unix datagram sockets. #35
* [BUGFIX] Sort source names #39

## 0.5.1 / 2022-11-17

* [CHANGE] Add armv7 container build #20

## 0.5.0 / 2022-10-22

* [FEATURE] Add System time (aka "current correction") metric #16
* [FEATURE] Add Root delay/dispersion metrics #17
* [FEATURE] Add multiple listeners and systemd socket listener #18

## 0.4.0 / 2022-10-06

* [FEATURE] Add `chrony_up` metric #14

## 0.3.1 / 2022-09-29

* [CHANGE] Update build to Go 1.19. #13
* [CHANGE] Update dependencies. #13

## 0.3.0 / 2022-07-11

* [CHANGE] Fix tracking info metric #8
* [CHANGE] Adjust tracking name parsing #9

## 0.2.0 / 2022-05-19

* [BUGFIX] Update Go client to reduce metrics cardinality
* [ENHANCEMENT] Add support for RefID #3

## 0.1.0 / 2022-03-20

Initial release.
