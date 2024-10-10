Tool to seed the relay with data
Sample call:
```bash
sudo apt-get install -y libvips-dev # dynamic linking
seeder --relays=wss://relay.damus.io/ --outputRelay=wss://localhost:9998 --profilesCount=1000 --threads=100 --perUser=100 --uploadKey=...
```
Params:
* relays = relay list to fetch data from (multiple can be passed like --relays=wss://relay.damus.io/ --relays=wss://strfry.iris.to/)
* outputRelay = relay to write data to
* profilesCount = count of profiles to fetch
* perUser = count of posts / articles to fetch for each user 
* uploadKey = imgbb free api key to host random webp images if user dont have any, if not provided - default image urls are rotated