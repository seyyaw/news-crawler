package cmd

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/thesoenke/news-crawler/scraper"
)

var itemsInputFile string
var contentOutDir string
var scrapeVerbose bool

var cmdScrape = &cobra.Command{
	Use:   "scrape",
	Short: "Scrape all provided articles",
	RunE: func(cmd *cobra.Command, args []string) error {
		if itemsInputFile == "" {
			return errors.New("Please provide a file with articles")
		}

		location, err := time.LoadLocation(timezone)
		if err != nil {
			return err
		}

		stat, err := os.Stat(itemsInputFile)
		if err != nil {
			return err
		}
		// Append current day to path when only received directory as input location
		if stat.IsDir() {
			// TODO check whether file for today exists
			dayLocation := time.Now().In(location)
			day := dayLocation.Format("2-1-2006")
			itemsInputFile = filepath.Join(itemsInputFile, day+".json")
		}
		contentScraper, err := scraper.New(itemsInputFile)
		if err != nil {
			return err
		}

		start := time.Now()
		contentScraper.Scrape(scrapeVerbose)
		articles := 0
		for _, feed := range contentScraper.Feeds {
			articles += len(feed.Items)
		}
		log.Printf("Successful: %d Failed: %d Time: %s", articles, contentScraper.Failures, time.Since(start))

		err = contentScraper.Store(contentOutDir, location)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	cmdScrape.PersistentFlags().StringVarP(&itemsInputFile, "file", "f", "", "Path to a JSON file with feed items")
	cmdScrape.PersistentFlags().StringVarP(&timezone, "timezone", "t", "Europe/Berlin", "Timezone for storing the feeds")
	cmdScrape.PersistentFlags().StringVarP(&contentOutDir, "out", "o", "out/content/", "Directory where to store the articles")
	cmdScrape.PersistentFlags().BoolVarP(&scrapeVerbose, "verbose", "v", false, "Verbose logging of scraper")
	RootCmd.AddCommand(cmdScrape)
}
