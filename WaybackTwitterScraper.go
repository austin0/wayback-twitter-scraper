package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	http "github.com/bogdanfinn/fhttp"
	"github.com/corona10/goimghdr"
	"github.com/gookit/color"
)

func main() {
	DrawTitle()            // Draw the title of the program
	inputUsername()        // Prompt user for Twitter username
	CreateDirectories()    // Create necessary directories for storing images
	LoadProxies()          // Load proxies from the proxies.txt file
	CreateStoredImageMap() // Create an in-memory map of stored images
	fetchWaybackPages()    // Fetch Wayback Machine cached pages
	parseImages()          // Parse images from the cached pages
	RemoveCommonItems()    // Remove previously downloaded images from the unprocessedImages list
	downloadImages()       // Download images from the Wayback Machine cache
	purgeCorrupted()       // Purge any corrupted images
	createReport()         // Create a report of the downloaded images
}

func inputUsername() {
	fmt.Print("\nEnter a Twitter username: ")
	fmt.Scanln(&TwitterUsername)

	for InvalidUsernameCheck() {
		fmt.Print("Enter a Twitter username: ")
		fmt.Scanln(&TwitterUsername)
	}
}

func fetchWaybackPages() {
	color.Cyan.Printf("Fetching list of Wayback Machine cached pages for profile: %s\n", TwitterUsername)

	var waybackResults [][]interface{}
	WaybackResultsURL = fmt.Sprintf("https://web.archive.org/web/timemap/json?url=twitter.com/%s&matchType=prefix", TwitterUsername)
	httpClient := GetProxyClient()
	req, err := http.NewRequest(http.MethodGet, WaybackResultsURL, nil)
	if err != nil {
		log.Println(err)
		return
	}

	for i := 0; i < 5; i++ {
		resp, err := httpClient.Do(req)
		if err != nil {
			color.Red.Printf("Retrying - Error fetching Wayback Machine results: %t", err)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}
		defer resp.Body.Close()
		if err := json.NewDecoder(resp.Body).Decode(&waybackResults); err != nil {
			color.Red.Println("Error decoding Wayback Machine results:", err)
			rotateClientProxy(httpClient)
			continue
		}
		break
	}

	for _, result := range waybackResults {
		pageURL := result[2].(string)
		if strings.Contains(pageURL, `http`) {
			PageUnprocessed = append(PageUnprocessed, pageURL)
		}
	}

	if len(PageUnprocessed) == 0 {
		color.Red.Printf("Found %d cached Wayback Machine pages - exiting\n", len(PageUnprocessed))
		os.Exit(0)
	} else {
		color.Cyan.Printf("Found %d cached Wayback Machine pages\n", len(PageUnprocessed))
	}
}

func parseImages() {
	color.LightGreen.Printf("\n=== Visiting the cached pages and checking for images\n")

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxThreads) // Limit to "MaxThreads" concurrent goroutines

	for len(PageUnprocessed) > 0 {
		wg.Add(1)
		var pageURL string

		PageUnprocessed, pageURL = Pop(PageUnprocessed)

		go func(pageURL string) { // Pass pageURL as an argument
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			combinedURL := WaybackPrefix + pageURL
			color.Gray.Printf("%s - Visiting %s to parse images\n", GetPageProgress(), pageURL)

			htmlContent, err := parseImagesWithRetry(combinedURL)
			switch err {
			case nil:
				PageProcessed = append(PageProcessed, pageURL)
				color.Green.Printf("%s - Successfully parsed %s\n", GetPageProgress(), pageURL)
			case http.ErrNotSupported:
				color.FgDarkGray.Printf("Skipping %s - not a valid page\n", pageURL)
				return
			default:
				fmt.Printf("Error parsing images from %s - %s\n", combinedURL, err)
				PageUnprocessed = append(PageUnprocessed, pageURL)
				return
			}

			for _, resource := range Resources {
				var resourceURLs []string
				switch resource {
				case "media":
					resourceURLs = MediaRegex.FindAllString(htmlContent, -1)
				case "profile":
					resourceURLs = addSizeSpread(ProfileRegex.FindAllString(htmlContent, -1))
				}
				ImageUnprocessed = RemoveDuplicates(append(ImageUnprocessed, resourceURLs...))
			}
		}(pageURL)
	}
	wg.Wait()

	color.Green.Printf("\nFound %d cached images for: %s\n", len(ImageUnprocessed), TwitterUsername)
}

func parseImagesWithRetry(combinedURL string) (string, error) {
	retry := 5
	httpClient := GetProxyClient()
	req, err := http.NewRequest(http.MethodGet, combinedURL, nil)
	if err != nil {
		return "TLS Client", err
	}

	for i := 0; i < retry; i++ {
		resp, err := httpClient.Do(req)
		if err != nil {
			color.Red.Printf("Error fetching page content: %s", err.Error())
			httpClient.CloseIdleConnections()
			rotateClientProxy(httpClient)
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode == 404 {
			return "404 - Not a valid page", http.ErrNotSupported
		}

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Error: HTTP request failed with status code %d\n", resp.StatusCode)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			color.Red.Printf("Error reading response body content for %s: %s\n", combinedURL, err)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}

		htmlContent := string(body)
		return htmlContent, nil
	}
	return "", fmt.Errorf("error fetching page content after %d retries", retry)
}

func downloadImageWithRetry(imageURL string, downloadPath string) string {
	retry := 5
	httpClient := GetProxyClient()
	req, err := http.NewRequest(http.MethodGet, imageURL, nil)
	if err != nil {
		log.Println(err)
	}

	for i := 0; i < retry; i++ {
		resp, err := httpClient.Do(req)
		if err != nil {
			color.Red.Printf("Retrying - Error fetching image: %s\n", err)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}

		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			color.Red.Printf("Retrying - Error reading image: %s\n", err)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}

		if resp.StatusCode == 404 && resp.Header.Get("Content-Type") == "text/html" {
			return "404 - Not an image resource"
		}

		if resp.StatusCode != http.StatusOK {
			color.Red.Printf("Retrying - Error fetching image: HTTP status %d\n", resp.StatusCode)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}

		file, err := os.Create(downloadPath)
		if err != nil {
			color.Red.Printf("Retrying - Error creating file: %s\n", err)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}
		defer file.Close()

		_, err = io.Copy(file, bytes.NewReader(bodyBytes))
		if err != nil {
			color.Red.Printf("Retrying - Error saving image: %s\n", err)
			rotateClientProxy(httpClient)
			httpClient.CloseIdleConnections()
			continue
		}
		return "Success"
	}
	color.Red.Printf("Aborting - Error downloading image after %d retries: %s\n", retry, imageURL)
	return ""
}

func downloadImages() {
	color.LightGreen.Println("Downloading indentified images from Wayback Machine Cache\n")

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxThreads) // Limit to "MaxThreads" concurrent goroutines

	for len(ImageUnprocessed) > 0 {
		wg.Add(1)
		var imageURL string
		var imageType string

		ImageUnprocessed, imageURL = Pop(ImageUnprocessed)

		if strings.Contains(imageURL, "media") {
			imageType = "media"
		} else {
			imageType = "profile"
		}

		go func(imageURL string, imageType string) {
			defer wg.Done()

			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			imageName := FilenameRegex.FindString(imageURL)
			combinedURL := WaybackPrefix + imageURL
			downloadPath := fmt.Sprintf("%s/%s/%s", UsernameLocation, imageType, imageName)

			resultString := downloadImageWithRetry(combinedURL, downloadPath)
			switch resultString {
			case "Success":
				TotalDownloads += 1
				ImageProcessed = append(ImageProcessed, imageURL)
				color.Green.Printf("%s - Saved %s\n", GetImageProgress(), imageURL)
				return
			case "404 - Not an image resource":
				color.FgDarkGray.Printf("Skipping %s - not a valid image file\n", imageURL)
				return
			default:
				color.Red.Printf("Error downloading image from %s - %s\n", combinedURL, resultString)
				ImageUnprocessed = append(ImageUnprocessed, imageURL)
				return
			}
		}(imageURL, imageType)
	}
	wg.Wait()

	color.Green.Printf("\nSaved %d images for username: %s\n", TotalDownloads, TwitterUsername)
}

func purgeCorrupted() {
	color.Gray.Printf("\nPurging any corrupted images in %s\n", UsernameLocation)

	images, err := filepath.Glob(fmt.Sprintf("%s/images/%s/*.jpg", HomeDirectory, TwitterUsername))
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

func createReport() {
	header := fmt.Sprintf(`=== Wayback Report - %s - %s`, TwitterUsername, GetCurrentDate())
	totalProcessed := fmt.Sprintf("Pages Parsed: %d | Images Proccesed: %d | Downloaded Images: %d", len(PageProcessed), len(ImageProcessed), TotalDownloads)
	pageString := ""
	for _, link := range PageProcessed {
		pageString += fmt.Sprintf("%s\n", link)
	}
	imageString := ""
	for _, link := range ImageProcessed {
		imageString += fmt.Sprintf("%s\n", link)
	}

	report := fmt.Sprintf("%s\n%s\n%s\n%s\n", header, totalProcessed, pageString, imageString)

	reportFile, err := os.Create(fmt.Sprintf("%s/%s-report.txt", UsernameLocation, GetCurrentDate()))
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

	color.Magenta.Printf("\nReport created: %s/%s-report.txt\n", UsernameLocation, GetCurrentDate())
}
