# wayback-twitter-scraper

Scrapes the [Internet Archive](https://web.archive.org/) for all content related to a given Twitter handle, archives the following:

- ~~Page HTML~~
- Image
- ~~Video~~

### Usage:

First, pull in any missing packages. Then run the program using one of the provided commands.

Pull in packages:
```
go get ./...
```

Run without building:
```
go run ./...
```

Build and run:
```
go build -o waybackScraper
./waybackScraper
```

#### Proxies

Proxies are supported for scraping profiles with a large amount of historical activity.

Proxies should be stored in a `proxies.txt` text file under the `proxies` directory. i.e. `proxies\proxies.txt`

Each individual proxy should use the following format:

`ip:port:username:password`
