package main

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/common-nighthawk/go-figure"
	"github.com/gookit/color"
)

func DrawTitle() {
	myFigure := figure.NewColorFigure("WaybackScraper", "poison", "blue", true)
	myFigure.Print()
}

func GetPWD() string {
	pwd, err := os.Getwd()
	if err != nil {
		color.Red.Println("Error getting PWD:", err)
		return ""
	}
	return pwd
}

func InvalidUsernameCheck() bool {
	if TwitterUsername == "" {
		fmt.Println(`"" - is not a valid username`)
		return true
	}

	valid := regexp.MustCompile(`^[a-zA-Z0-9_]{1,15}$`).MatchString(TwitterUsername)
	if !valid {
		fmt.Println("Username can only contain alphanumeric characters and underscores (1-15 characters)")
		return true
	}

	return false
}

func GetCurrentDate() string {
	currentTime := time.Now()
	dateString := currentTime.Format("2006-01-02")
	return dateString
}

func addSizeSpread(profileURLs []string) []string {
	for _, profileURL := range profileURLs {
		baseProfile := truncateString(profileURL, 65)
		profileURLs = append(profileURLs, fmt.Sprintf(`%s.jpg`, baseProfile))
		profileURLs = append(profileURLs, fmt.Sprintf(`%s_400x400.jpg`, baseProfile))
		profileURLs = append(profileURLs, fmt.Sprintf(`%s_normal.jpg`, baseProfile))
		profileURLs = append(profileURLs, fmt.Sprintf(`%s_bigger.jpg`, baseProfile))
	}
	return profileURLs
}

func GetPageProgress() string {
	return fmt.Sprintf("[%d / %d]", len(PageProcessed), (len(PageUnprocessed) + len(PageProcessed)))
}

func GetImageProgress() string {
	return fmt.Sprintf("[%d / %d]", len(ImageProcessed), (len(ImageUnprocessed) + len(ImageProcessed)))
}

func Pop(slice []string) ([]string, string) {
	if len(slice) == 0 {
		return slice, ""
	}
	popped := slice[len(slice)-1]
	slice = slice[:len(slice)-1]
	return slice, popped
}

func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Removes duplicate items from a slice
func RemoveDuplicates(inputSlice []string) []string {
	uniqueSlice := make([]string, 0)
	tempMap := make(map[string]bool)

	for _, item := range inputSlice {
		tempMap[item] = true
	}

	for item := range tempMap {
		uniqueSlice = append(uniqueSlice, item)
	}

	return uniqueSlice
}

// Removes the objects stored in StoredImageMap from the unprocessedImageSlice
func RemoveCommonItems(parsedImages []string, storedImages map[string]bool) {
	tempSlice := []string{}

	for _, item := range parsedImages {
		// regex item for just filename
		itemFilename := FilenameRegex.FindString(item)
		if !storedImages[itemFilename] {
			tempSlice = append(tempSlice, item)
		}
	}
	color.Magenta.Printf("Filtered %d previously downloaded images - %s\n", len(parsedImages)-len(tempSlice), UsernameLocation)
	ImageUnprocessed = tempSlice
}
