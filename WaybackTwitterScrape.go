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
	"time"

	"github.com/corona10/goimghdr"
	"github.com/gookit/color"
)

var (
	pageCache         = make(map[string]int)
	mediaCache        = make(map[string]int)
	profileCache      = make(map[string]int)
	waybackResultsURL string
	saveLocation      string
	twitterAccount    string
	waybackPrefix     = "https://web.archive.org/web/20200126021126if_/"
	mediaRegex        = regexp.MustCompile(`https://pbs.twimg.com/media/[A-Za-z0-9_.\-]+.jpg`)
	profileRegex      = regexp.MustCompile(`https://pbs.twimg.com/profile_images/[0-9]+/[A-Za-z0-9_.\-]+.jpg`)
	filenameRegex     = regexp.MustCompile(`[A-Za-z0-9_.]+.jpg`)
	homeDirectory     string
	proxies           []Proxy
	mediaCacheMutex   sync.Mutex
	profileCacheMutex sync.Mutex
	storedImagesMutex sync.Mutex
	storedImages      = make(map[string]bool)
)

type Proxy struct {
	IP       string
	Port     string
	Username string
	Password string
}

func main() {
	getPWD()

	fmt.Printf("\nLoading proxies: %s/proxies/proxies.txt\n", homeDirectory)
	loadProxies(homeDirectory, &proxies)
	fmt.Printf("\nLoaded %d proxies from file\n", len(proxies))

	for !validUsername() {
		fmt.Print("Enter Twitter username: ")
		fmt.Scanln(&twitterAccount)
	}

	waybackResultsURL = fmt.Sprintf("https://web.archive.org/web/timemap/json?url=https://twitter.com/%s&matchType=prefix", twitterAccount)
	saveLocation = fmt.Sprintf("%s/images/%s", homeDirectory, twitterAccount)

	if _, err := os.Stat(saveLocation); os.IsNotExist(err) {
		os.MkdirAll(saveLocation, os.ModePerm)
	}

	initStoredImages()

	fmt.Printf("Fetching list of WaybackMachine cached pages for profile: %s\n", twitterAccount)
	fetchWaybackPages(waybackResultsURL)
	color.Yellow.Printf("Found %d cached pages\n", len(pageCache))

	fmt.Println("Visiting the cached pages and checking for images")
	parseImages(saveLocation)
	fmt.Printf("Saved %d cached images\n", len(mediaCache)+len(profileCache))

	fmt.Printf("Purging any corrupted images in %s\n", saveLocation)
	purgeCorrupted(twitterAccount)

	fmt.Printf("Report created: %s/report.txt\n", saveLocation)
	createReport(saveLocation)
}

func validUsername() bool {
	if twitterAccount == "" {
		fmt.Println(`"" - is not a valid username`)
		return false
	}

	valid := regexp.MustCompile(`^[a-zA-Z0-9_]+{1,15}$`).MatchString(twitterAccount)
	if !valid {
		fmt.Println("Username can only contain alphanumeric characters and underscores")
		return false
	}

	return true
}

func getPWD() {
	pwd, err := os.Getwd()
	if err != nil {
		color.Red.Println("Error getting PWD:", err)
		return
	}

	homeDirectory = pwd
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

func initStoredImages() {
	mediaDir := fmt.Sprintf(`%s/images/%s/media`, homeDirectory, twitterAccount)
	profileDir := fmt.Sprintf(`%s/images/%s/profile`, homeDirectory, twitterAccount)

	addImagesFromDirectory(mediaDir)
	addImagesFromDirectory(profileDir)
}

func addImagesFromDirectory(directoryPath string) {
	paths, err := filepath.Glob(directoryPath)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for _, path := range paths {
		storedImagesMutex.Lock()
		storedImages[path] = true
		storedImagesMutex.Unlock()
	}
}

func imageAlreadyExists(imageURL string) bool {
	storedImagesMutex.Lock()
	defer storedImagesMutex.Unlock()
	return storedImages[imageURL]
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
			pageCache[pageLink] = (len(pageCache) + 1)
		}
	}
}

func parseImages(saveLocation string) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 5) // Limit to 5 concurrent goroutines

	for pageLink := range pageCache {
		wg.Add(1)

		go func(pageLink string) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			combinedURL := waybackPrefix + pageLink
			fmt.Printf("[%d / %d] - Visiting %s to parse images\n", pageCache[pageLink], len(pageCache), pageLink)

			resp, err := http.Get(combinedURL)
			if err != nil {
				color.Red.Println("Error fetching page content:", err)
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				color.Red.Printf("Error: HTTP request failed with status code %d\n", resp.StatusCode)
				return
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				color.Red.Println("Error reading page content:", err)
				return
			}

			htmlContent := string(body)

			mediaURLs := mediaRegex.FindAllString(htmlContent, -1)
			profileURLs := profileRegex.FindAllString(htmlContent, -1)
			imageURLs := append(mediaURLs, profileURLs...)

			for _, imageURL := range imageURLs {
				if isCached(imageURL) {
					continue
				}
				downloadImageWithRetry(imageURL, saveLocation)
			}
		}(pageLink)
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

func downloadImageWithRetry(imageLink, saveLocation string) {
	retry := 3
	for i := 0; i < retry; i++ {
		err := downloadImage(imageLink, saveLocation)
		if err == nil {
			return
		}
		time.Sleep(1 * time.Second) // Wait before retrying
	}
	color.Red.Printf("Error downloading image after %d retries: %s\n", retry, imageLink)
}

func downloadImage(imageLink, saveLocation string) error {
	imageType := ""
	var imageCache *map[string]int

	if strings.Contains(imageLink, "media") {
		mediaCacheMutex.Lock()
		defer mediaCacheMutex.Unlock()
		mediaCache[imageLink] = len(mediaCache) + 1
		imageType = "media"
		imageCache = &mediaCache
	} else {
		profileCacheMutex.Lock()
		defer profileCacheMutex.Unlock()
		profileCache[imageLink] = len(profileCache) + 1
		imageType = "profile"
		imageCache = &profileCache
	}

	imageName := filenameRegex.FindString(imageLink)
	combinedURL := waybackPrefix + imageLink
	localPath := fmt.Sprintf("%s/%s/%s", saveLocation, imageType, imageName)

	color.Yellow.Printf("[%d / %d] - Fetching %s image %s\n", (*imageCache)[imageLink], len(*imageCache), imageType, imageLink)

	if imageAlreadyExists(imageName) {
		color.Green.Printf("[%d / %d] - Image %s found in cache\n", (*imageCache)[imageLink], len(*imageCache), imageLink)
		return nil
	}

	randomProxy := proxies[rand.Intn(len(proxies))]
	proxyString := fmt.Sprintf("http://%s:%s@%s:%s", randomProxy.Username, randomProxy.Password, randomProxy.IP, randomProxy.Port)

	proxyURL, err := url.Parse(proxyString)
	if err != nil {
		return fmt.Errorf("error parsing proxy URL: %s", err)
	}

	proxyClient := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyURL),
		},
	}

	resp, err := proxyClient.Get(combinedURL)
	if err != nil {
		return fmt.Errorf("error fetching image: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error fetching image: HTTP status %d", resp.StatusCode)
	}

	file, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("error creating file: %s", err)
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("error saving image: %s", err)
	}

	color.Green.Printf("Saved %s - %s\n", imageType, localPath)
	return nil
}

func purgeCorrupted(twitterAccount string) {
	images, err := filepath.Glob(fmt.Sprintf("%s/images/%s/*.jpg", homeDirectory, twitterAccount))
	if err != nil {
		color.Red.Println("Error listing image files:", err)
		return
	}

	purgedCounter := 0

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
				purgedCounter += 1
			}
		}
	}

	color.Green.Println(`No corrupted images found - success!`)
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
