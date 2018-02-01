package main

import(
	"flag"
	"runtime"
	"fmt"
	"time"
	"log"
	"github.com/PuerkitoBio/goquery"
	"strings"
	"io/ioutil"
	"os"
	fp "path/filepath"
)

var url = flag.String("url", "www.baidu.com", "Start websit")
var downloadDir = flag.String("dir", "./Downloads", "Path to store download file")
var sUrl = flag.String("furl", "" , "Keyword to filter webpage")
var sPic = flag.String("fpic", "", "Keyword to filter picture")
var sParent = flag.String("fparent", "", "Keyword to filter father webpage")
var imgAttr = flag.String("img", "src", "Name of picture suffix, for example: data-original")
var minSize = flag.Int("size", 150,"Minimum picture size(kb)")
var maxSize = flag.Int("no", 20, "Number of crawler pictures")
var recursive = flag.Bool("re", true, "Whetherr recuresize current webpage")

var seen = History{m: map[string]bool{}}
var count = new(Counts)
var urlChan = make(chan *URL, 99999999)
var picChan = make(chan *URL, 99999999)
var done = make(chan int)

var goPicNum = make(chan int, 20)
var HOST string

func main(){
	runtime.GOMAXPROCS(4)
	flag.Parse()

	if *url == ""{
		fmt.Println("Use -h or --help to get help!")
		return
	}

	fmt.Printf("Start: %v MinSize: %v MaxSize: %v Recursive: %v Dir: %v <img>attribution: %v\n", *url, *minSize, *maxSize, *recursive, *downloadDir, *imgAttr)
	fmt.Printf("Filter: URL: %v Pic: %v ParentPage: %v\n", *sUrl, *sPic, *sParent)
	u := NewURL(*url, nil, *downloadDir)
	HOST = u.Host
	fmt.Println(HOST)
	urlChan <- u
	seen.Add(u.Url)
	time.Sleep(time.Second * 3)

	go HandleHTML()
	go HandlePic()

	<-done
	log.Printf("Download %v pictures", count.Value("download"))
	log.Printf("END")
}

func HandleHTML(){
	for {
		select {
		case u := <-urlChan:
			res := u.Get()
			if res == nil {
				log.Println("HTML response is nil! following process will not execute")
				continue
			}

			doc, err := goquery.NewDocumentFromResponse(res)
			if err != nil{
				log.Println(fmt.Sprintf("error is %v",err))
				continue
			}

			if doc == nil {
				log.Println("doc is nil return")
				continue
			}
			if *recursive {
				parseLinks(doc, u, urlChan, picChan)
			}

			parsePics(doc, u, picChan)
			count.Inc("page")
			log.Printf("Crawler %v webpages %s", count.Value("page"), u.Url)
		default:
			log.Println("Crawler schedule is empty, completed")
			done <- 1
		}
	}

	//runtime.Gosched()
}

func HandlePic(){
	for u := range picChan {
		u := u
		goPicNum <- 1
		go func(){
			defer func() {
				<- goPicNum
				if count.Value("download") >= *maxSize {
					done <- 1
				}
			}()

			var data []byte
			res := u.Get()
			if res == nil {
				log.Println("HTML response is nil! following process will not execute.")
				return
			}

			defer res.Body.Close()
			count.Inc("pic")
			if res.StatusCode == 200 {
				body := res.Body
				//Get img in byte
				data, _ = ioutil.ReadAll(body)
				body.Close()
			} else {
				log.Println(res.StatusCode)
				time.Sleep(time.Second * 3)
				return
			}

			if len(data) == 0 {
				return
			}

			if len(data) >= *minSize * 1000 {
				cwd, e := os.Getwd()
				if e != nil {
					cwd = "."
				}

				picFile := fp.Join(cwd, u.FilePath)
				if exists(picFile) {
					return
				}

				picDir := fp.Dir(picFile)
				if !exists(picDir) {
					mkdirs(picDir)
				}

				f, e := os.Create(picFile)
				fatal(e)
				defer f.Close()
				_, e = f.Write(data)
				fatal(e)
				count.Inc("download")
				log.Printf("Crawler %v picture, current picture size: %v kb", count.Value("download"), len(data)/1000)
			} else {
				log.Printf("Crawler %v picture, current picture size: %v kb", count.Value("pic"), len(data)/1000)
			}
		}()

		//runtime.Gosched()
	}
}

func parseLinks(doc *goquery.Document, parent *URL, urlChan, picChan chan *URL){
	doc.Find("a").Each(func(i int, s *goquery.Selection){
		url, ok := s.Attr("href")
		url = strings.Trim(url, " ")
		if ok {
			if strings.HasPrefix(url, "#") || strings.HasPrefix(strings.ToLower(url), "javascript") || url == "" {
				return
			} else {
				new := NewURL(url, parent, *downloadDir)
				if seen.Has(new.Url){
					log.Printf("Url is crawlered， ignore %v", new.Url)
				} else {
					seen.Add(new.Url)
					if !IsPic(new.Url) {
						if !strings.Contains(new.Url, HOST) {
							log.Printf("Url is beyond the limited， ignore %v", new.Url)
						}
					}
				}

				if !strings.Contains(new.Path, *sUrl){
					return
				}

				if IsPic(url) {
					picChan <- new
					log.Printf("New <a> Pic: %s", url)
				} else {
					select{
					case urlChan <- new:
						if strings.Contains(url, "http") {
							log.Printf("New PAGE: %s", url)
						} else {
							log.Printf("New PAGE: %s --> %s", url, new.Url)
						}
					default:
						log.Println("url channel is full!!!")
						time.Sleep(time.Second * 3)
					}
				}
			}
		}
	})
}

func parsePics(doc *goquery.Document, parent *URL, picChaen chan *URL) {
	doc.Find("img").Each(func(i int, s *goquery.Selection){
		url, ok := s.Attr(*imgAttr)
		url = strings.Trim(url, " ")
		if ok {
			if strings.HasPrefix(strings.ToLower(url), "data") || url == "" {
				return
			} else {
				new := NewURL(url, parent, *downloadDir)
				if seen.Has(new.Url) {
					log.Printf("Picture is crawlered, ignore %v", new.Url)
				} else {
					seen.Add(new.Url)
					if !strings.Contains(parent.Path, *sParent) {
						log.Printf("Parent page is not satisfy the keyword, ignore %v", new.Url)
						return
					}

					if !strings.Contains(new.Path, *sPic) {
						log.Printf("Picture is not satisfy the keyword, ignore %v", new.Url)
						return
					}

					if exists(new.FilePath) {
						log.Printf("Picture is exist, ignore %v", new.Url)
						return
					}

					picChan <- new
					log.Printf("new <img> PIC: %s", url)
				}
			}
		}
	})
}