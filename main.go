package main

import (
	"embed"
	"encoding/gob"
	"fmt"
	"index/suffixarray"
	"log"
	"os"
	"path/filepath"
	"time"

	"cogentcore.org/core/core"
	"cogentcore.org/core/events"
	"github.com/dslipak/pdf"
)

type PDF struct {
	Path         string
	Filename     string
	Numpages     int
	PageContents []pageData
}

type pageData struct {
	PageNumber int
	PageText   string
	pageIndex  *suffixarray.Index
}

type result struct {
	Matchfile PDF
	Matches   []PageResult
}

type PageResult struct {
	PageFile   string
	PageNumber int
	Index      []int
}

type State struct {
	Page      int
	File      string
	Highlight string
}

var searchHits []result
var currentResults *[]result

var DocID int
var DocIDptr *int

var PageID int
var PageIDptr *int

var ActiveSearchTerm string
var ActiveSearchTermptr *string

var ActiveState State
var ActiveStateptr *State

var err error

// needed for release and distribution, bot during development and debugging
//
//go:embed index.html pagesnip.html previewsnip.html resultsnip.html htmx.min.js
var staticFiles embed.FS

func main() {
	var minuteList []PDF
	currentResults = &searchHits
	DocIDptr = &DocID
	PageIDptr = &PageID
	ActiveSearchTermptr = &ActiveSearchTerm
	ActiveStateptr = &ActiveState

	fmt.Println("scanning for all minutes:")
	beforescan := time.Now()
	minuteList, err := scanforminutes()
	check(err)
	afterscan := time.Since(beforescan)
	fmt.Println(fmt.Sprint(len(minuteList)) + " files indexed. It took " + fmt.Sprint(afterscan))

	b := core.NewBody()
	core.NewButton(b).SetText("Hello, World!")
	tf := core.NewTextField(b)

	tf.OnFocusLost(func(e events.Event) {
		result := seekCollection(tf.Text(), minuteList)
		fmt.Println(len(result))
		tf.SetFocus()
	})

	b.RunMainWindow()

}

func seekCollection(searchterm string, collection []PDF) (results []result) {
	for _, file := range collection {
		var result result
		result.Matchfile = file

		for i := 0; i < file.Numpages; i++ {

			var resultPage PageResult
			resultPage.PageFile = file.Path
			resultPage.PageNumber = i + 1
			resultPage.Index = file.PageContents[i].pageIndex.Lookup([]byte(searchterm), -1)
			if len(resultPage.Index) == 0 {
				continue
			}
			result.Matches = append(result.Matches, resultPage)
			fmt.Print("found term: at")
			fmt.Printf("%v \n", resultPage.Index)
		}

		if len(result.Matches) == 0 {
			continue
		}
		results = append(results, result)
	}
	return results
}

func check(e error) {
	if e != nil {
		log.Println(e)
	}
}

func ListFiles(ext string, path string) ([]string, error) {
	var fileList []string
	// Read all the file recursively
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}
	err := filepath.Walk(path, func(file string, f os.FileInfo, err error) error {
		if filepath.Ext(file) == ext {
			fileList = append(fileList, file)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileList, nil
}

func scanforminutes() (collection []PDF, err error) {

	if _, err := os.Stat("cache.gob"); !os.IsNotExist(err) {

		var index_from_cache []PDF
		cacheFile, err := os.Open("cache.gob")
		check(err)
		cacheDecoder := gob.NewDecoder(cacheFile)
		err = cacheDecoder.Decode(&index_from_cache)
		check(err)

		for i := 0; i < len(index_from_cache); i++ {

			for j := 0; j < len(index_from_cache[i].PageContents); j++ {
				index_from_cache[i].PageContents[j].pageIndex = suffixarray.New([]byte(index_from_cache[i].PageContents[j].PageText))

			}
		}
		return index_from_cache, nil
	}

	minutespath := filepath.Join("data")
	var cleanlist []string
	cleanlist, err = ListFiles(".pdf", minutespath)
	check(err)
	for _, minutes := range cleanlist {
		var content PDF
		r, err := pdf.Open(minutes)
		// remember close file
		if err != nil {
			fmt.Println("error opening file")
			return []PDF{}, err
		}
		content.Path = minutes
		content.Filename = filepath.Base(minutes)
		content.Numpages = r.NumPage()
		content.PageContents = getIndexedPages(r)
		collection = append(collection, content)
	}

	file, err := os.Create("cache.gob")
	cacheEncoder := gob.NewEncoder(file)
	err = cacheEncoder.Encode(collection)

	check(err)
	return collection, nil
}

func getIndexedPages(r *pdf.Reader) (pages []pageData) {
	for i := 1; i <= r.NumPage(); i++ {
		var page pageData
		b, err := r.Page(i).GetPlainText(nil)

		page.PageText = b
		page.pageIndex = suffixarray.New([]byte(b))
		page.PageNumber = i + 1

		check(err)

		pages = append(pages, page)
	}
	return
}
func getPages(r *pdf.Reader) (pages []pdf.Page) {
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		pages = append(pages, page)
	}
	return
}

func getStringFromIndex(data []byte, index int) string {
	var start, end int
	for i := index - 1; i >= 0; i-- {
		if data[i] == 0 {
			start = i + 1
			break
		}
	}
	for i := index + 1; i < len(data); i++ {
		if data[i] == 0 {
			end = i
			break
		}
	}
	return string(data[start:end])
}

func getStringAround(data []byte, index int) string {
	var start, end int
	start = index - 100
	if start < 0 {
		start = 0
	}
	end = index + 100
	if end > len(data) {
		end = len(data)
	}

	return string(data[start:end])
}
