package main

import (
	"bytes"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"sync"
	"time"
)

type category struct {
	categoryName string
	categoryUrl  string
}

const baseUrl = "https://home.meishichina.com/"

func spiderCategoryPage(wg *sync.WaitGroup, channel chan []byte, client *resty.Client) {
	defer wg.Done()

	log.Info().Msg("开始爬取recipe-type.html页")
	response, err := client.R().EnableTrace().Get("/recipe-type.html")
	if err != nil {
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
		log.Err(err).Msg("recipe-type.html 页面爬取错误!")
	}

	if response.StatusCode() == 200 {
		channel <- response.Body()
	} else {
		log.Warn().
			Int("statusCode", response.StatusCode()).
			Str("functionName", "spiderCategoryPage(wg *sync.WaitGroup, channel chan []byte, client *resty.Client)").
			Str("url", client.BaseURL+"/recipe-type.html").
			Msg("页面爬取错误!")
	}
}

func parseCategoryPage(wg *sync.WaitGroup, categoryPageChannel chan []byte, categoryChannel chan category) {
	defer wg.Done()
	responseByte := <-categoryPageChannel
	reader := bytes.NewReader(responseByte)

	document, err := goquery.NewDocumentFromReader(reader)
	if err != nil {
		log.Err(err).
			Str("functionName", "parseCategoryPage(wg *sync.WaitGroup, channel chan []byte)").
			Str("parseContent", string(responseByte)).
			Msg("页面解析失败!")
	}

	document.Find("body > div.wrap > div > div > div > ul > li").Each(func(i int, selection *goquery.Selection) {
		categoryName, categoryNameExists := selection.Find("a").Attr("title")

		categoryURL, categoryURLExists := selection.Find("a").Attr("href")
		if categoryNameExists {
			// 如果title中没有分类名称则在跟content再获取一次分类名称
			categoryName = selection.Find("a").Text()
			if categoryName == "" {
				log.Warn().
					Msg("超链接名称元素不存在")
			}
		}

		if categoryURLExists {
			log.Warn().
				Msg("超链接URL元素不存在")
		}

		categoryChannel <- category{
			categoryName: categoryName,
			categoryUrl:  categoryURL,
		}
	})
}

func main() {
	httpClient := resty.New()
	// 十次重试 每次间隔一秒
	httpClient.SetRetryCount(10)
	httpClient.SetRetryWaitTime(time.Second)
	httpClient.SetBaseURL(baseUrl)

	// 分类页面爬取channel 如果channel中有数据确没协程拿则会使程序阻塞
	categoryPageChannel := make(chan []byte)

	categoryChannel := make(chan category)

	var wg sync.WaitGroup

	wg.Add(1)
	go spiderCategoryPage(&wg, categoryPageChannel, httpClient)
	wg.Add(1)
	go parseCategoryPage(&wg, categoryPageChannel, categoryChannel)

	wg.Wait()
}
