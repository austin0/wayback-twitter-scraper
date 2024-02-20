package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/corona10/goimghdr"
	"github.com/gookit/color"
)

var (
	pageCache         = make(map[string]bool)
	mediaCache        = make(map[string]bool)
	profileCache      = make(map[string]bool)
	waybackResultsURL string
	saveLocation      string
	twitterAccount    string
	waybackPrefix     = "https://web.archive.org/web/20200126021126if_/"
	mediaRegex        = regexp.MustCompile(`https://pbs.twimg.com/media/[A-Za-z0-9_.\-]+.jpg`)
	profileRegex      = regexp.MustCompile(`https://pbs.twimg.com/profile_images/[0-9]+/[A-Za-z0-9_.\-]+.jpg`)
	filenameRegex     = regexp.MustCompile(`[A-Za-z0-9_.]+.jpg`)
	homeDirectory     = "C:/Users/austi/Desktop/wayback"
	proxies           []Proxy
	mediaCacheMutex   sync.Mutex
	profileCacheMutex sync.Mutex
)

type Proxy struct {
	IP       string
	Port     string
	Username string
	Password string
}

func main() {
	fmt.Printf("\nLoading proxies: %s/proxies/proxies.txt\n", homeDirectory)
	loadProxies(homeDirectory, &proxies)
	fmt.Printf("\nLoaded %d proxies from file\n", len(proxies))

	fmt.Print("Enter Twitter username: ")
	fmt.Scanln(&twitterAccount)

	waybackResultsURL = fmt.Sprintf("https://web.archive.org/web/timemap/json?url=https://twitter.com/%s&matchType=prefix", twitterAccount)
	saveLocation = fmt.Sprintf("%s/%s/images", homeDirectory, twitterAccount)

	if _, err := os.Stat(saveLocation); os.IsNotExist(err) {
		os.MkdirAll(saveLocation, os.ModePerm)
	}

	fmt.Printf("Fetching list of WaybackMachine cached pages for profile: %s\n", twitterAccount)
	fetchWaybackPages(waybackResultsURL)
	fmt.Printf("Found %d cached pages\n", len(pageCache))

	fmt.Println("Visiting the cached pages and checking for images")
	parseImages(saveLocation)
	fmt.Printf("Saved %d cached images\n", len(mediaCache)+len(profileCache))

	fmt.Printf("Purging any corrupted images in %s\n", saveLocation)
	purgeCorrupted(twitterAccount)

	fmt.Printf("Report created: %s/report.txt\n", saveLocation)
	createReport(saveLocation)
}

func loadProxies(homeDirectory string, proxies *[]Proxy) {
	proxyFile, err := os.Open(fmt.Sprintf("%s/proxies/proxies.txt", homeDirectory))
	if err != nil {
		color.Red.Println("Error opening proxy file:", err)
		os.Exit(1)
	}
	defer proxyFile.Close()

	scanner := bufio.NewScanner(proxyFile)
	for scanner.Scan() {
		proxyParts := strings.Split(scanner.Text(), ":")
		if len(proxyParts) != 4 {
			color.Yellow.Println("Invalid proxy format:", scanner.Text())
			continue
		}

		*proxies = append(*proxies, Proxy{
			IP:       proxyParts[0],
			Port:     proxyParts[1],
			Username: proxyParts[2],
			Password: proxyParts[3],
		})
	}

	if err := scanner.Err(); err != nil {
		color.Red.Println("Error reading proxy file:", err)
	}
}

func fetchWaybackPages(waybackResultsURL string) {
	resp, err := http.Get(waybackResultsURL)
	if err != nil {
		color.Red.Println("Error fetching WaybackMachine results:", err)
		return
	}
	defer resp.Body.Close()

	var waybackResults [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&waybackResults); err != nil {
		color.Red.Println("Error decoding WaybackMachine results:", err)
		return
	}

	for _, result := range waybackResults {
		pageLink, _ := result[2].(string)
		if strings.Contains(pageLink, `http`) {
			pageCache[pageLink] = true
		}
	}
}

func parseImages(saveLocation string) {
	semaphore := make(chan struct{}, 10) // Limit to 10 concurrent goroutines
	var wg sync.WaitGroup

	for pageLink := range pageCache {
		combinedURL := waybackPrefix + pageLink

		fmt.Printf("Visiting %s to parse images\n", pageLink)

		resp, err := http.Get(combinedURL)
		if err != nil {
			color.Red.Println("Error fetching page content:", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Error: HTTP request failed with status code %d\n", resp.StatusCode)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			color.Red.Println("Error reading page content:", err)
			continue
		}

		htmlContent := string(body)

		mediaURLs := mediaRegex.FindAllString(htmlContent, -1)
		profileURLs := profileRegex.FindAllString(htmlContent, -1)
		imageURLs := append(mediaURLs, profileURLs...)

		for _, imageURL := range imageURLs {
			if isCached(imageURL) {
				continue
			}

			semaphore <- struct{}{} // Acquire semaphore

			wg.Add(1)
			go func(imageURL string) {
				defer wg.Done()
				defer func() { <-semaphore }() // Release semaphore
				downloadImage(imageURL, saveLocation)
			}(imageURL)
		}
	}

	wg.Wait()
}

func isCached(imageURL string) bool {
	mediaCacheMutex.Lock()
	defer mediaCacheMutex.Unlock()

	if _, ok := mediaCache[imageURL]; ok {
		return true
	}

	profileCacheMutex.Lock()
	defer profileCacheMutex.Unlock()

	if _, ok := profileCache[imageURL]; ok {
		return true
	}

	return false
}

func downloadImage(imageLink, saveLocation string) {
	imageType := ""

	if strings.Contains(imageLink, "media") {
		mediaCacheMutex.Lock()
		defer mediaCacheMutex.Unlock()
		mediaCache[imageLink] = true
		imageType = "media"
	} else {
		profileCacheMutex.Lock()
		defer profileCacheMutex.Unlock()
		profileCache[imageLink] = true
		imageType = "profile"
	}

	color.Green.Printf("Parsed %s image: %s\n", imageType, imageLink)

	imageName := filenameRegex.FindString(imageLink)
	combinedURL := waybackPrefix + imageLink
	localPath := fmt.Sprintf("%s/%s", saveLocation, imageName)

	fmt.Printf("Fetching %s image %s, storing at %s\n", imageType, imageLink, localPath)

	randomProxy := proxies[rand.Intn(len(proxies))]
	proxyString := fmt.Sprintf("http://%s:%s@%s:%s", randomProxy.Username, randomProxy.Password, randomProxy.IP, randomProxy.Port)

	proxyURL, err := url.Parse(proxyString)
	if err != nil {
		color.Red.Println("Error parsing proxy URL:", err)
		return
	}

	proxyClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	resp, err := proxyClient.Get(combinedURL)
	if err != nil {
		color.Red.Printf("Error fetching image: %s\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return
	}

	if resp.StatusCode != http.StatusOK {
		color.Red.Printf("Error fetching image: HTTP status %d\n", resp.StatusCode)
		return
	}

	file, err := os.Create(localPath)
	if err != nil {
		color.Red.Printf("Error creating file: %s\n", err)
		return
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		color.Red.Printf("Error saving image: %s\n", err)
		return
	}

	color.Green.Printf("Saved %s - %s\n", imageType, localPath)
}

func purgeCorrupted(twitterAccount string) {
	images, err := filepath.Glob(fmt.Sprintf("%s/%s/images/*.jpg", homeDirectory, twitterAccount))
	if err != nil {
		color.Red.Println("Error listing image files:", err)
		return
	}

	for _, image := range images {
		if _, err := os.Stat(image); err != nil {
			color.Red.Printf("Error checking image file: %s\n", err)
			continue
		}
		if _, err := os.Stat(image); err == nil {
			filetype, _ := goimghdr.What(image)
			if filetype != "jpeg" {
				if err := os.Remove(image); err != nil {
					color.Red.Printf("Error removing corrupted image: %s\n", err)
					continue
				}
				color.Green.Printf("Removed corrupted image: %s\n", image)
			}
		}
	}
}

func createReport(saveLocation string) {
	header := fmt.Sprintf(`=== Wayback Report - %s`, twitterAccount)
	totalProcessed := fmt.Sprintf("Pages Parsed: %d | Images Saved: %d", len(pageCache), len(mediaCache))
	pageCacheString := ""
	for link := range pageCache {
		pageCacheString += fmt.Sprintf("%s\n", link)
	}
	mediaCacheString := ""
	for link := range mediaCache {
		mediaCacheString += fmt.Sprintf("%s\n", link)
	}
	profileCacheString := ""
	for link := range profileCache {
		profileCacheString += fmt.Sprintf("%s\n", link)
	}

	report := fmt.Sprintf("%s\n%s\n%s\n%s\n%s", header, totalProcessed, pageCacheString, mediaCacheString, profileCacheString)

	reportFile, err := os.Create(fmt.Sprintf("%s/report.txt", saveLocation))
	if err != nil {
		color.Red.Println("Error creating report file:", err)
		return
	}
	defer reportFile.Close()

	_, err = reportFile.WriteString(report)
	if err != nil {
		color.Red.Println("Error writing to report file:", err)
		return
	}
}
