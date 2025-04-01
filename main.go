package main

import (
	"embed"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"index/suffixarray"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"text/template"
	"time"

	"github.com/dslipak/pdf"
	// webview "github.com/webview/webview_go"
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

	http.HandleFunc("/{$}", indexHandler)

	searchHandler := func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(len(minuteList))
		r.ParseMultipartForm(10 << 20)
		seekerTerm := r.FormValue("term")

		fmt.Println(r.FormValue("context"))

		fmt.Println("searching collection for: " + seekerTerm)
		beforesearch := time.Now()
		*currentResults = seekCollection(seekerTerm, minuteList)
		aftersearch := time.Since(beforesearch)
		*ActiveSearchTermptr = seekerTerm

		ActiveStateptr.Highlight = seekerTerm

		fmt.Println("there was a total of " + fmt.Sprint(len(searchHits)) + " hits for the term: " + seekerTerm + ". It took " + aftersearch.String())
		resultTemp := template.Must(template.ParseFiles("resultsnip.html"))
		err := resultTemp.Execute(w, searchHits)
		check(err)
	}
	http.Handle("/", http.FileServer(http.Dir("."))) //husk en generel fileserver til css, scripts, ressourcer osv
	http.HandleFunc("/search", searchHandler)

	http.HandleFunc("/expandresult/{id}", expandHandler)
	http.HandleFunc("/getpreview/{id}", previewHandler)
	http.HandleFunc("/getCurrentState", getStateHandler)

	fmt.Println("starting server at localhost:1337 for testing purposes. Press Ctrl+c to cancel.")

	http.ListenAndServe(":1337", nil)
	/*
	   debug := true
	   w := webview.New(debug)

	   	if w == nil {
	   		log.Fatalln("Failed to load webview.")
	   	}

	   defer w.Destroy()
	   w.SetTitle("Minimal webview example")
	   w.SetSize(800, 600, webview.HintNone)
	   w.Navigate("http://127.0.0.1:1337")
	   w.Run()
	*/
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("index.html"))
	fmt.Println("Endpoint \"/\" hit")
	err := tmpl.Execute(w, nil)
	check(err)
}

func expandHandler(w http.ResponseWriter, r *http.Request) {

	key := r.PathValue("id")
	*DocIDptr, err = strconv.Atoi(key)
	check(err)
	fmt.Println(searchHits[DocID].Matchfile.Filename)
	ActiveStateptr.File = searchHits[DocID].Matchfile.Path

	pageTemplate := template.Must(template.ParseFiles("pagesnip.html"))
	err = pageTemplate.Execute(w, searchHits[DocID].Matches)

}

func previewHandler(w http.ResponseWriter, r *http.Request) {

	key := r.PathValue("id")
	*PageIDptr, err = strconv.Atoi(key)
	check(err)
	pageTemplate := template.Must(template.ParseFiles("previewsnip.html"))
	ActiveStateptr.Page = searchHits[DocID].Matches[PageID].PageNumber
	fmt.Println(ActiveState.Page)
	err = pageTemplate.Execute(w, searchHits[DocID].Matches[PageID])

}

func getStateHandler(w http.ResponseWriter, r *http.Request) {

	fmt.Println(ActiveState)
	json.NewEncoder(w).Encode(ActiveState)

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
