package main

import (
	"cogentcore.org/core/core"
	"cogentcore.org/core/events"
	"cogentcore.org/core/icons"
	"cogentcore.org/core/styles"
	"cogentcore.org/core/styles/units"
	"encoding/gob"
	"fmt"
	"github.com/dslipak/pdf"
	"github.com/gen2brain/go-fitz"
	"image"
	"index/suffixarray"
	"log"
	"os"
	"path/filepath"
	"time"
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

var err error

func main() {
	var ActiveState State
	ActiveState.Highlight = "startup"
	ActiveStateptr := &ActiveState

	var minuteList []PDF
	currentResults = &searchHits

	var resultStrings []string
	resultSptr := &resultStrings

	var pageStrings []string
	pageStringsPtr := &pageStrings

	fmt.Println("scanning for all minutes:")
	beforescan := time.Now()
	minuteList, err := scanforminutes()
	check(err)
	afterscan := time.Since(beforescan)
	fmt.Println(fmt.Sprint(len(minuteList)) + " files indexed. It took " + fmt.Sprint(afterscan))

	//TOP LEVEL
	b := core.NewBody()
	b.SetTitle("libui float")

	//TOP BAR
	topbar := core.NewFrame(b)

	topbar.Styler(func(s *styles.Style) {
		s.CenterAll()
	})

	searchEntry := core.NewTextField(topbar)

	searchButton := core.NewButton(topbar).SetText("Search")
	searchButton.SetIcon(icons.Search)

	stateViewer := core.Bind(ActiveStateptr, core.NewForm(topbar))
	stateViewer.SetReadOnly(true)

	//COLUMNS
	columns := core.NewFrame(b)

	column1 := core.NewList(columns)
	column1.SetSlice(resultSptr)
	column1.SetReadOnly(true)
	column1.Styler(func(s *styles.Style) {
		s.Min.Set(units.Em(20))
	})

	var c1index int
	column1.BindSelect(&c1index)

	column2 := core.NewList(columns)
	column2.SetSlice(pageStringsPtr)
	column2.SetReadOnly(true)
	column2.Styler(func(s *styles.Style) {
		s.Min.Set(units.Em(30))
		s.Grow.Set(1, 1)
	})
	var c2index int
	var c3image image.Image
	c3PTR := &c3image
	column2.BindSelect(&c2index)

	column1.OnChange(func(e events.Event) {
		*pageStringsPtr = pageResultAdapter((*currentResults)[c1index].Matches)
		ActiveStateptr.File = (*currentResults)[c1index].Matchfile.Filename
		stateViewer.Update()
		column2.Update()

	})

	c3image = pageExtractor("test.pdf", 1)
	column3Frame := core.NewFrame(columns)
	column3Frame.Styler(func(s *styles.Style) {
		s.Direction = styles.Column
	})
	c3NavBar := core.NewFrame(column3Frame)
	c3NavBar.Styler(func(s *styles.Style) {
		s.Direction = styles.Row
	})
	c3NavBar.Styler(func(s *styles.Style) {
		s.CenterAll()
	})
	c3PrevPage := core.NewButton(c3NavBar).SetText("Previous page")
	c3PrevPage.SetIcon(icons.ArrowLeft)
	c3OpenFile := core.NewButton(c3NavBar).SetText("Open File")
	c3OpenFile.SetIcon(icons.Open)
	c3NextPage := core.NewButton(c3NavBar).SetText("Next page")
	c3NextPage.SetIcon(icons.ArrowRight)

	column3 := core.NewImage(column3Frame)

	column3.Styler(func(s *styles.Style) {
		s.Min.Set(units.Em(80))
		s.Grow.Set(1, 1)
	})

	column3.SetImage(c3image)
	column2.OnChange(func(e events.Event) {
		ActiveStateptr.Page = searchHits[c1index].Matches[c2index].PageNumber
		stateViewer.Update()
		*c3PTR = pageExtractor(searchHits[c1index].Matchfile.Path, searchHits[c1index].Matches[c2index].PageNumber)

		column3.SetImage(c3image)
		column3.Update()
	})

	//handle global events
	b.OnFirst(events.Types(events.KeyDown), func(e events.Event) {
		if e.KeyChord() == "Control+F" {
			fmt.Println("ctrl+f pressed")
			searchEntry.SetFocus()
		}
	})
	topbar.OnFirst(events.Types(events.KeyDown), func(e events.Event) {
		if e.KeyChord() == "Control+F" {
			fmt.Println("ctrl+f pressed")
			searchEntry.SetFocus()
		}
	})

	columns.OnFirst(events.Types(events.KeyDown), func(e events.Event) {
		if e.KeyChord() == "Control+F" {
			fmt.Println("ctrl+f pressed")
			searchEntry.SetFocus()
		}
	})
	submitSearch := func(e events.Event) {

		if e.KeyChord() == "ReturnEnter" || e.Type() == events.Click {
			*currentResults = seekCollection(searchEntry.Text(), minuteList)
			fmt.Println(len(*currentResults))
			if len(*currentResults) == 0 {
				ActiveStateptr.Page = 0
				ActiveStateptr.File = "none"
				ActiveStateptr.Highlight = ""
				stateViewer.Update()
				return
			}

			*resultSptr = resultAdapter(*currentResults)
			*pageStringsPtr = pageResultAdapter((*currentResults)[0].Matches)
			ActiveStateptr.Highlight = searchEntry.Text()
			ActiveStateptr.File = (*currentResults)[0].Matches[0].PageFile
			ActiveStateptr.Page = (*currentResults)[0].Matches[0].PageNumber
			column1.Update()
			column2.Update()
			stateViewer.Update()

		}
	}

	searchButton.OnClick(submitSearch)

	searchEntry.On(events.Types(events.KeyDown), submitSearch)

	b.RunMainWindow()

}

func pageExtractor(file string, page int) image.Image {
	fmt.Printf("extracting page %v from %v \n", page, file)
	doc, err := fitz.New(file)
	if err != nil {
		panic(err)
	}

	defer doc.Close()

	img, err := doc.Image(page)

	//m := resize.Resize(800, 0, img, resize.Lanczos3)
	if err != nil {
		panic(err)
	}
	return img
}

func resultAdapter(results []result) (output []string) {
	for _, result := range results {
		line := fmt.Sprintf("%v with %v hits", result.Matchfile.Filename, len(result.Matches))
		output = append(output, line)
	}
	return output
}
func pdfAdapter(minutes []PDF) (output []string) {
	for _, minute := range minutes {
		line := fmt.Sprintf("File: %v with %v hits", minute.Filename, minute.Numpages)
		output = append(output, line)
	}
	return output
}

func pageResultAdapter(pageResults []PageResult) (output []string) {
	for _, pageResult := range pageResults {
		line := fmt.Sprintf("Page number: %v", pageResult.PageNumber)
		output = append(output, line)
	}
	return output

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
