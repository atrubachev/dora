# dora - bmc discovery asset database

A tool to build a dynamic database of you datacenter assets

## How to run

Small installations / Dev Setup:

git clone github.com/bmc-toolbox/dora
cd dora
go build -tags="gingonic" -ldflags="-s -w"
./dora --config dora-simple.yaml server

Kea config file to load subnets:

kea-dhcp4.json
{
    "Dhcp4": {
          "subnet4": [
                {
                "id": 16,
                "option-data": [
                    {
                        "data": "192.168.0.1",
                        "name": "routers"
                    },
                    {
                        "data": "bmc.example.com",
                        "name": "domain-name"
                    },
                    {
                        "data": "bmc.example.com example.com",
                        "name": "domain-search"
                    }
                ],
                "pools": [
                    {
                        "pool": "192.168.0.10 - 192.168.0.200"
                    }
                ],
                "subnet": "192.168.0.1/24"
                }
          ]
    }
}

## Requirements

Database - Any compatible with gorm

#### Acknowledgment

dora was originally developed for [Booking.com](http://www.booking.com).
With approval from [Booking.com](http://www.booking.com), the code and
specification were generalized and published as Open Source on github, for
which the authors would like to express their gratitude.
