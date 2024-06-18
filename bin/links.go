package main

import (
	"bufio"
	"crypto/md5"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

func getDoc(sourceUrl string) (*goquery.Document, error) {
	resp, err := http.Get(sourceUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Failed to get document: %s", sourceUrl))
	}
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func loadUrlCount(sourceUrl string) (map[string]int, error) {
	urlCount := map[string]int{}
	filepath := md5_(sourceUrl)
	if _, err := os.Stat(filepath); errors.Is(err, os.ErrNotExist) {
		return urlCount, nil
	}
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " ", 2)
		if len(parts) == 2 {
			if count, err := strconv.Atoi(parts[1]); err == nil {
				urlCount[parts[0]] = count
			}
		}
	}
	return urlCount, nil
}

func loadSourceUrls(filepath string) ([]string, error) {
	urls := []string{}
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		urls = append(urls, scanner.Text())
	}
	return urls, nil
}

func md5_(sourceUrl string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte(sourceUrl)))
}

func saveUrlCount(sourceUrl string, urlCount map[string]int) error {
	filepath := md5_(sourceUrl)
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	for u, count := range urlCount {
		if count > 0 {
			file.WriteString(fmt.Sprintf("%s %d\n", u, count))
		}
	}
	return nil
}

type link struct {
	url  string
	text string
}

func getLinks(sourceUrl string, links chan link) {
	defer close(links)
	doc, err := getDoc(sourceUrl)
	if err != nil {
		fmt.Println(err)
		return
	}
	f := func(i int, s *goquery.Selection) bool {
		linkUrl, _ := s.Attr("href")
		url_, err := url.Parse(linkUrl)
		if err != nil {
			return false
		}
		return len(url_.Hostname()) > 0 && (strings.HasPrefix(linkUrl, "https://") || strings.HasPrefix(linkUrl, "http://"))
	}
	doc.Find("html a").FilterFunction(f).Each(
		func(_ int, tag *goquery.Selection) {
			linkUrl, ok := tag.Attr("href")
			if ok {
				linkText := strings.TrimSpace(tag.Text())
				if len(linkText) == 0 {
					linkText = linkUrl
				}
				links <- link{linkUrl, linkText}
			}
		})
}

type httpHandler struct {
	sourceUrls []string
	urlCount   map[string]map[string]int
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "<html><head></head><body>\n")
	links := make([]chan link, len(h.sourceUrls))
	for i, sourceUrl := range h.sourceUrls {
		links[i] = make(chan link)
		go getLinks(sourceUrl, links[i])
	}
	for i, sourceUrl := range h.sourceUrls {
		fmt.Fprintf(w, "<h3>%s</h3>\n", sourceUrl)
		urlCount := map[string]int{}
		newLinkFound := false
		for link := range links[i] {
			uc, ok := h.urlCount[sourceUrl]
			if ok {
				count, ok := uc[link.url]
				if ok {
					urlCount[link.url] = count + 1
				} else {
					_, ok := urlCount[link.url]
					if !ok {
						fmt.Fprintf(w, "<p><a href=\"%s\">%s</a></p>\n", link.url, link.text)
						urlCount[link.url] = 1
						newLinkFound = true
					}
				}
			}
		}
		if !newLinkFound {
			fmt.Fprintf(w, "<p>No new links found</p>")
		}
		h.urlCount[sourceUrl] = urlCount
		if err := saveUrlCount(sourceUrl, urlCount); err != nil {
			fmt.Println(err)
		}
	}
	fmt.Fprintf(w, "</body></html>")
}

func main() {
	var err error
	port := 8080
	if len(os.Args) == 2 {
	} else if len(os.Args) == 3 {
		port, err = strconv.Atoi(os.Args[2])
		if err != nil {
			fmt.Println("links sourceurls <port>")
			fmt.Println(err)
			return
		}
	} else {
		fmt.Println("links sourceUrls <port>")
		return
	}
	fmt.Printf("http://localhost:%d\n", port)
	sourceUrls, err := loadSourceUrls(os.Args[1])
	if err != nil {
		fmt.Println(err)
		return
	}
	handler := &httpHandler{sourceUrls, map[string]map[string]int{}}
	for _, sourceUrl := range sourceUrls {
		urlCount, err := loadUrlCount(sourceUrl)
		if err != nil {
			fmt.Println(err)
			return
		}
		handler.urlCount[sourceUrl] = urlCount
	}
	http.Handle("/", handler)
	portString := fmt.Sprintf(":%d", port)
	if err := http.ListenAndServe(portString, nil); err != nil {
		fmt.Println(err)
		return
	}
}
