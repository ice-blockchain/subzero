Tool to seed the relay with data
Sample call:
```bash
seeder --relays=wss://relay.damus.io/ --outputRelay=wss://localhost:9998 --profilesCount=1000 --threads=100 --perUser=100
```
Params:
* relays = relay list to fetch data from
* outputRelay = relay to write data to
* profilesCount = count of profiles to fetch
* perUser = count of posts / articles to fetch for each user 