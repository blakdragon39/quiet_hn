package main

import (
	"errors"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gophercises/quiet_hn/hn"
)

var (
	cache []item
	cacheExpiration time.Time
)

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		stories, err := getCachedStories(numStories)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		data := templateData{
			Stories: stories,
			Time:    time.Now().Sub(start),
		}
		err = tpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Failed to process the template", http.StatusInternalServerError)
			return
		}
	})
}

func getCachedStories(numStories int) ([]item, error) {
	if time.Now().Sub(cacheExpiration) < 0 {
		return cache, nil
	}

	stories, err := getTopStories(numStories)
	if err != nil {
		return nil, err
	}

	cache = stories
	cacheExpiration = time.Now().Add(10 * time.Second)
	return cache, nil
}

func getTopStories(numStories int) ([]item, error) {
	var client hn.Client
	ids, err := client.TopItems()
	if err != nil {
		return nil, errors.New("Failed to load top stories")

	}

	var stories []item
	current := 0
	for len(stories) < numStories {
		need := (numStories - len(stories)) * 5 / 4 //get a few extra stories just in case
		stories = append(stories, getStories(ids[current:current+need])...)
		current += need
	}

	return stories[:numStories], nil
}

func getStories(ids []int) []item {
	type result struct {
		index int
		item item
		err error
	}

	resultCh := make(chan result)

	for i := 0; i < len(ids); i++ {
		go func(i int) {
			var client hn.Client
			hnItem, err := client.GetItem(ids[i])
			if err != nil {
				resultCh <- result{index: i, err: err}
			} else {
				resultCh <- result{index: i, item: parseHNItem(hnItem)}
			}
		}(i)
	}

	var results []result
	for i := 0; i < len(ids); i++ {
		results = append(results, <-resultCh)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].index < results[j].index
	})

	var stories []item
	for _, res := range results {
		if res.err != nil {
			continue
		}

		if isStoryLink(res.item) {
			stories = append(stories, res.item)
		}
	}

	return stories
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
