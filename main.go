package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
	"gopkg.in/ini.v1"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Delicious struct {
	ID            int        `json:"id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	Image         string     `json:"image"`
	Materials     []Material `json:"materials"`
	Flavor        string     `json:"flavor"`
	Craft         string     `json:"craft"`
	TimeConsuming string     `json:"time_consuming"`
	Difficulty    string     `json:"difficulty"`
	Categories    []string   `json:"categories"`
	Ware          string     `json:"ware"`
	Steps         []Step     `json:"steps"`
}

type Material struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Type   string `json:"type"`
}

type Step struct {
	Image   string `json:"image,omitempty"`
	Content string `json:"content,omitempty"`
}

var httpClient *resty.Client
var baseUrl string
var crawlerNum int
var errorCount = 0

// Init 初始化方法，初始化一些全局变量以及HTTP对象等
func Init() {
	Cfg, err := ini.Load("conf/application.ini")
	if err != nil {
		log.Fatalf("Fail to parse 'conf/application.ini':%v", err)
	}
	section := Cfg.Section("server")
	baseUrl = section.Key("baseUrl").MustString("")
	crawlerNum = section.Key("crawlerNum").MustInt()
	proxyUrl := section.Key("proxyUrl").MustString("")
	debug := section.Key("debug").MustBool()
	timeout := section.Key("timeout").MustInt()

	httpClient = resty.New()
	httpClient.SetProxy(proxyUrl)
	httpClient.SetDebug(debug)
	httpClient.SetTimeout(time.Duration(timeout) * time.Second)
}

func start() {
	idChannel := make(chan int)
	lock := sync.Mutex{}

	go t(idChannel, &lock)

	for i := 0; i < crawlerNum; i++ {
		idChannel <- i
	}

	close(idChannel)
}

func t(idChan chan int, lock *sync.Mutex) {
	for id := range idChan {
		// 发送http请求
		url := fmt.Sprintf(baseUrl, id)

		responseContent, err := httpClient.R().Get(url)
		if err != nil {

		}

		if responseContent.StatusCode() == 404 {
			log.Println("page is 404, url: " + url)
		}

		if responseContent.StatusCode() != 200 && responseContent.StatusCode() != 404 {
			responseContent, err = nil, nil
			log.Println("try again, url:" + url)
			// 睡眠一秒 重试一次
			time.Sleep(1 * time.Second)
			responseContent, err = httpClient.R().Get(url)
			log.Println("Try again to complete, code:" + strconv.Itoa(responseContent.StatusCode()) + "url: " + url)
		}

		if responseContent.StatusCode() == 200 {
			ParseResponseContent(responseContent.String())
		}
	}

}
func ParseResponseContent(response string) error {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response))
	delicious := new(Delicious)
	id, exists := doc.Find("#recipe_id").Attr("value")
	if exists {
		idToInt, err := strconv.Atoi(id)
		if err != nil {
			log.Println("string转int错误")
			return err
		}
		delicious.ID = idToInt
	} else {
		log.Println("id不存在。")
	}

	title := doc.Find("#recipe_title").Text()
	delicious.Title = title

	desc := doc.Find("#block_txt1").Text()

	delicious.Description = desc

	image, exits := doc.Find("#recipe_De_imgBox > a > img").Attr("src")
	if exits {
		delicious.Image = image
	} else {
		log.Println("image不存在。")
	}
	materials := make([]Material, 0)
	doc.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > fieldset").
		Each(func(i int, selection *goquery.Selection) {
			materialsType := selection.Find("legend").Text()
			selection.Find("div > ul > li").
				Each(func(i int, selection *goquery.Selection) {
					// 分两种情况 一种情况是有a链接的 一种情况是没a链接的 如果第一种情况获取不到则使用第二种情况获取
					name := selection.Find("span.category_s1 b").Text()
					amount := selection.Find("span.category_s2").Text()
					material := Material{
						Name:   name,
						Amount: amount,
						Type:   materialsType,
					}
					materials = append(materials, material)
				})
		})
	delicious.Materials = materials
	doc.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > div.recipeCategory_sub_R.mt30.clear > ul > li").
		Each(func(i int, selection *goquery.Selection) {
			text := selection.Find("span.category_s1 > a").Text()
			if i == 0 {
				delicious.Flavor = text
			} else if i == 1 {
				delicious.Craft = text
			} else if i == 2 {
				delicious.TimeConsuming = text
			} else if i == 3 {
				delicious.Difficulty = text
			}
		})

	categories := make([]string, 0)
	doc.Find("a.vest").Each(func(i int, selection *goquery.Selection) {
		category := selection.Text()

		categories = append(categories, category)
	})
	delicious.Categories = categories

	doc.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > .mt16").
		Each(func(i int, selection *goquery.Selection) {
			content := replaceAllSpaceAndNewLine(selection.Text())
			if strings.HasPrefix(content, "使用的厨具：") {
				split := strings.Split(content, "使用的厨具：")
				if len(split) == 2 {
					delicious.Ware = split[1]
				}
			}
		})

	steps := make([]Step, 0)
	doc.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > div.recipeStep > ul > li").
		Each(func(i int, selection *goquery.Selection) {
			step := Step{
				Image:   "",
				Content: "",
			}
			stepImage, e := selection.Find("div.recipeStep_img > img").Attr("src")
			if e {
				step.Image = stepImage
			} else {
				log.Println("步骤图片未找到.")
			}

			stepContent := selection.Find("div.recipeStep_word").Text()
			regex, regexError := regexp.Compile("^[0-9]+")
			if regexError != nil {
			}
			stepContent = regex.ReplaceAllString(stepContent, "")
			step.Content = stepContent

			steps = append(steps, step)
		})

	delicious.Steps = steps

	writeFile(delicious)

	return err
}

func writeFile(delicious *Delicious) {
	// 写文件前替换一下
}

func replaceAllSpaceAndNewLine(content string) string {
	return strings.ReplaceAll(strings.ReplaceAll(content, "\t", ""), "\n", "")
}

func main() {

	//// 初始化一系列对象
	Init()
	start()
}
