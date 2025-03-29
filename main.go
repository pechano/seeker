package main

import (
	"bytes"
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
	webview "github.com/webview/webview_go"
)

type PDF struct {
	Path         string
	Filename     string
	plaintext    string
	numpages     int
	pages        []pdf.Page
	pageContents []pageData
	resultchan   chan int
	index        *suffixarray.Index
	buf          []byte
	minuteType   string
}

type pageData struct {
	pageNumber int
	pageText   string
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

var searchHits []result
var currentResults *[]result

var DocID int
var DocIDptr *int

var PageID int
var PageIDptr *int
var err error

func main() {

	var minuteList []PDF
	currentResults = &searchHits
	DocIDptr = &DocID
	PageIDptr = &PageID

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

		fmt.Println("there was a total of " + fmt.Sprint(len(searchHits)) + " hits for the term: " + seekerTerm + ". It took " + aftersearch.String())
		resultTemp := template.Must(template.ParseFiles("resultsnip.html"))
		err := resultTemp.Execute(w, searchHits)
		check(err)
	}
	http.Handle("/", http.FileServer(http.Dir("."))) //husk en generel fileserver til css, scripts, ressourcer osv
	http.HandleFunc("/search", searchHandler)

	http.HandleFunc("/expandresult/{id}", expandHandler)
	http.HandleFunc("/getpreview/{id}", previewHandler)

	fmt.Println("starting server at localhost:1337 for testing purposes. Press Ctrl+c to cancel.")

	go http.ListenAndServe(":1337", nil)
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
	pageTemplate := template.Must(template.ParseFiles("pagesnip.html"))
	err = pageTemplate.Execute(w, searchHits[DocID].Matches)

}

func previewHandler(w http.ResponseWriter, r *http.Request) {

	key := r.PathValue("id")
	*PageIDptr, err = strconv.Atoi(key)
	check(err)
	pageTemplate := template.Must(template.ParseFiles("previewsnip.html"))
	err = pageTemplate.Execute(w, searchHits[DocID].Matches[PageID])

}
func seekCollection(searchterm string, collection []PDF) (results []result) {
	for _, file := range collection {
		var result result
		result.Matchfile = file

		for i := 0; i < file.numpages; i++ {

			var resultPage PageResult
			resultPage.PageFile = file.Path
			resultPage.PageNumber = i + 1
			resultPage.Index = file.pageContents[i].pageIndex.Lookup([]byte(searchterm), -1)
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
		var buf bytes.Buffer
		b, err := r.GetPlainText()
		if err != nil {
			fmt.Println("error creating buffer")
			return []PDF{}, err
		}

		buf.ReadFrom(b)
		index := suffixarray.New(buf.Bytes())
		content.Path = minutes
		content.Filename = filepath.Base(minutes)
		content.buf = buf.Bytes()
		content.plaintext = buf.String()
		content.numpages = r.NumPage()
		content.pages = getPages(r)
		content.pageContents = getIndexedPages(r)
		content.index = index
		collection = append(collection, content)
	}
	return collection, nil
}

func getIndexedPages(r *pdf.Reader) (pages []pageData) {
	for i := 1; i <= r.NumPage(); i++ {
		var page pageData
		b, err := r.Page(i).GetPlainText(nil)

		page.pageText = b
		page.pageIndex = suffixarray.New([]byte(b))
		page.pageNumber = i + 1

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
func readPages(r []pdf.Page) (pageContents []string) {
	for _, page := range r {
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			fmt.Print(err)
		}
		pageContents = append(pageContents, pageText)
	}
	return
}

func (p *PDF) seeker(searchterm string) {

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
