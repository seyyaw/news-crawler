package scraper

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/thesoenke/news-crawler/feedreader"
	"gopkg.in/cheggaaa/pb.v1"
	"gopkg.in/olivere/elastic.v5"
)

const (
	userAgent = "Mozilla/5.0 (X11; Linux x86_64; rv:53.0) Gecko/20100101 Firefox/53.0"
)

type Scraper struct {
	Feeds         []feedreader.Feed
	Articles      int
	Failures      int
	ElasticClient *elastic.Client
}

type Article struct {
	FeedItem *feedreader.FeedItem
	HTML     string
}

// New creates a scraper instance
func New(feedsFile string) (Scraper, error) {
	scraper := Scraper{}
	esClient, err := NewElasticClient()
	if err != nil {
		return scraper, err
	}

	scraper.ElasticClient = esClient
	feeds, err := scraper.loadFeeds(feedsFile)
	if err != nil {
		return scraper, err
	}

	scraper.Feeds = feeds
	return scraper, nil
}

// Scrape downloads the content of the provide list of urls
func (scraper *Scraper) Scrape(outDir string, dayTime *time.Time, verbose bool) error {
	wg := sync.WaitGroup{}
	queue := make(chan *feedreader.FeedItem)
	errChan := make(chan error)
	articleChan := make(chan *Article)
	numItems := 0
	for _, feed := range scraper.Feeds {
		numItems += len(feed.Items)
	}

	if !verbose {
		// prevents "Unsolicited response" log messages from http package when encountering buggy webserver
		log.SetOutput(ioutil.Discard)
	}

	bar := pb.StartNew(numItems)
	scraper.startWorker(&wg, queue, articleChan, errChan, verbose)
	go scraper.fillWorker(queue, scraper.Feeds)

	failures := 0
	for i := 0; i < numItems; i++ {
		select {
		case article := <-articleChan:
			err := article.Write(outDir, dayTime)
			if err != nil {
				log.SetOutput(os.Stderr)
				return err
			}

			err = scraper.index(article)
			if err != nil {
				log.SetOutput(os.Stderr)
				return err
			}
		case err := <-errChan:
			if ferr, ok := err.(*FetchError); ok {
				scraper.logError(ferr)
			}
			failures++
		}
		bar.Increment()
	}

	close(queue)
	wg.Wait()
	bar.Finish()
	log.SetOutput(os.Stderr)

	scraper.Articles = numItems
	scraper.Failures = failures

	return nil
}

func (scraper *Scraper) startWorker(wg *sync.WaitGroup, queue chan *feedreader.FeedItem, articleChan chan *Article, errChan chan error, verbose bool) {
	concurrencyLimit := 100

	for worker := 0; worker < concurrencyLimit; worker++ {
		wg.Add(1)

		go func(verbose bool) {
			defer wg.Done()

			for item := range queue {
				article := &Article{
					FeedItem: item,
				}

				err := article.Fetch()
				if err != nil {
					if verbose {
						fmt.Printf("Failed to fetch %s %s\n", item.URL, err)
					}
					errChan <- err
					continue
				}

				err = article.Extract()
				if err != nil {
					if verbose {
						fmt.Printf("Failed to extract %s %s\n", item.URL, err)
					}
					errChan <- err
					continue
				}

				articleChan <- article
			}
		}(verbose)
	}
}

func (scraper *Scraper) fillWorker(queue chan *feedreader.FeedItem, feeds []feedreader.Feed) {
	items := make([]*feedreader.FeedItem, 0)
	for _, feed := range feeds {
		items = append(items, feed.Items...)
	}

	shuffle(items)
	for _, item := range items {
		queue <- item
	}
}

func (scraper *Scraper) loadFeeds(path string) ([]feedreader.Feed, error) {
	articlesFile, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var feeds []feedreader.Feed
	err = json.Unmarshal(articlesFile, &feeds)
	return feeds, err
}

func shuffle(items []*feedreader.FeedItem) {
	for i := range items {
		j := rand.Intn(i + 1)
		items[i], items[j] = items[j], items[i]
	}
}
