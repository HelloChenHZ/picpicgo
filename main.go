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

var url = flag.String("url", "www.baidu.com", "起始网址")
var downloadDir = flag.String("dir", "./Downloads", "自定义存放文件的路径")
var sUrl = flag.String("furl", "" , "自定义过滤网页链接的关键字")
var sPic = flag.String("fpic", "", "自定义过滤图片链接的关键字")
var sParent = flag.String("fparent", "", "自定义过滤图片父页面链接须包含的关键字")
var imgAttr = flag.String("img", "src", "自定义图片属性名称， 如data-original")
var minSize = flag.Int("size", 150,"最小图片大小 单位kb")
var maxSize = flag.Int("no", 20, "需要爬取的有效图片数量")
var recursive = flag.Bool("re", true, "是否需要递归当前页面链接")

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
	log.Printf("图片统计：下载%v", count.Value("download"))
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

			//goquery会主动关闭res.Body
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
			log.Printf("当前爬取了 %v个网页 %s", count.Value("page"), u.Url)
		default:
			log.Println("待爬取队列为空，爬取完成")
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
				log.Printf("图片统计： 下载%v 当前图片大小: %v kb", count.Value("download"), len(data)/1000)
			} else {
				log.Printf("爬取%v 当前图片大小: %v kb", count.Value("pic"), len(data)/1000)
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
					log.Printf("链接已爬取， 忽略 %v", new.Url)
				} else {
					seen.Add(new.Url)
					if !IsPic(new.Url) {
						if !strings.Contains(new.Url, HOST) {
							log.Printf("链接已超出本站， 忽略 %v", new.Url)
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
					log.Printf("图片已爬取， 忽略 %v", new.Url)
				} else {
					seen.Add(new.Url)
					if !strings.Contains(parent.Path, *sParent) {
						log.Printf("父页面不满足过滤关键词， 忽略 %v", new.Url)
						return
					}

					if !strings.Contains(new.Path, *sPic) {
						log.Printf("不包含图片过滤关键词， 忽略 %v", new.Url)
						return
					}

					if exists(new.FilePath) {
						log.Printf("图片已存在， 忽略 %v", new.Url)
						return
					}

					picChan <- new
					log.Printf("new <img> PIC: %s", url)
				}
			}
		}
	})
}