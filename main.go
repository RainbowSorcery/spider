package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
	"gopkg.in/ini.v1"
	"log"
	"os"
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
var fileWrite *bufio.Writer
var wg = sync.WaitGroup{}

// Init 初始化方法，初始化一些全局变量以及HTTP对象等
func Init() {
	Cfg, err := ini.Load("conf/application.ini")
	if err != nil {
		panic(err)
	}
	section := Cfg.Section("server")
	baseUrl = section.Key("baseUrl").MustString("")
	crawlerNum = section.Key("crawlerNum").MustInt()
	proxyUrl := section.Key("proxyUrl").MustString("")
	debug := section.Key("debug").MustBool()
	timeout := section.Key("timeout").MustInt()
	outputPath := section.Key("outputPath").MustString("")
	outputFileName := section.Key("outputFileName").MustString("")

	mkdirErr := os.MkdirAll(outputPath, 0644)
	if mkdirErr != nil {
		panic(mkdirErr)
	}
	file, err := os.OpenFile(outputPath+outputFileName, os.O_WRONLY|os.O_CREATE, 0666)
	fileWrite = bufio.NewWriter(file)

	httpClient = resty.New()
	httpClient.SetProxy(proxyUrl)
	httpClient.SetDebug(debug)
	httpClient.SetTimeout(time.Duration(timeout) * time.Second)
}

func start() {
	var rwMutex = new(sync.RWMutex)
	wg.Add(1)
	defer wg.Done()

	errorChan := make(chan error)
	go errorHandle(errorChan, rwMutex)

	idChannel := make(chan int)
	go run(idChannel, errorChan)

	for i := 0; i < crawlerNum; i++ {
		idChannel <- i
	}

	close(idChannel)
}

// 执行主要程序
func run(idChan chan int, errorChan chan error) {
	for id := range idChan {
		// 发送http请求
		url := fmt.Sprintf(baseUrl, id)
		responseContent, err := httpClient.R().Get(url)
		if err != nil {
			errorChan <- err
			log.Fatalf("发送HTTP请求错误，错误信息：%v", err)
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
			if err != nil {
				errorChan <- err
			}
			log.Printf("Try again to complete, code:%d, %s", responseContent.StatusCode(), "url: "+url)
		}

		if responseContent.StatusCode() == 200 {
			parseError := ParseResponseContent(responseContent.String())
			if parseError != nil {
				errorChan <- parseError
			}
		}
	}

}

// ParseResponseContent 解析响应内容
func ParseResponseContent(response string) error {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(response))
	if err != nil {
		log.Fatalf("文档创建失败, 错误信息:%v", err)
		return err
	}
	delicious := new(Delicious)
	id, exists := doc.Find("#recipe_id").Attr("value")
	if exists {
		idToInt, idToIntError := strconv.Atoi(id)
		if idToIntError != nil {
			log.Println("string转int错误")
			return idToIntError
		}
		delicious.ID = idToInt
	} else {
		log.Println("id不存在。")
	}

	title := doc.Find("#recipe_title").Text()
	delicious.Title = replaceAllSpaceAndNewLine(title)

	desc := doc.Find("#block_txt1").Text()

	delicious.Description = replaceAllSpaceAndNewLine(desc)

	image, exits := doc.Find("#recipe_De_imgBox > a > img").Attr("src")
	if exits {
		delicious.Image = replaceAllSpaceAndNewLine(image)
	} else {
		log.Println("image不存在。")
	}
	materials := make([]Material, 0)
	doc.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > fieldset").
		Each(func(i int, selection *goquery.Selection) {
			materialsType := replaceAllSpaceAndNewLine(selection.Find("legend").Text())
			selection.Find("div > ul > li").
				Each(func(i int, selection *goquery.Selection) {
					// 分两种情况 一种情况是有a链接的 一种情况是没a链接的 如果第一种情况获取不到则使用第二种情况获取
					name := replaceAllSpaceAndNewLine(selection.Find("span.category_s1 b").Text())
					amount := replaceAllSpaceAndNewLine(selection.Find("span.category_s2").Text())
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
			text := replaceAllSpaceAndNewLine(selection.Find("span.category_s1 > a").Text())
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
		category := replaceAllSpaceAndNewLine(selection.Text())

		categories = append(categories, category)
	})
	delicious.Categories = categories

	doc.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > .mt16").
		Each(func(i int, selection *goquery.Selection) {
			content := replaceAllSpaceAndNewLine(selection.Text())
			if strings.HasPrefix(content, "使用的厨具：") {
				split := strings.Split(content, "使用的厨具：")
				if len(split) == 2 {
					delicious.Ware = replaceAllSpaceAndNewLine(split[1])
				}
			}
		})

	steps := make([]Step, 0)
	var regexError error = nil
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
				log.Println("步骤图片未找到, Id:" + strconv.Itoa(delicious.ID))
			}

			stepContent := selection.Find("div.recipeStep_word").Text()
			regex, regexErr := regexp.Compile("^[0-9]+")
			if regexErr != nil {
				regexError = regexErr
				log.Printf("正则表达式执行错误，错误信息：%v", regexError)
			}
			stepContent = regex.ReplaceAllString(stepContent, "")
			step.Content = replaceAllSpaceAndNewLine(stepContent)

			steps = append(steps, step)
		})

	if regexError != nil {
		return regexError
	}

	delicious.Steps = steps
	writeErr := writeFile(delicious)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

// 写文件
func writeFile(delicious *Delicious) error {
	log.Printf("开始写文件, Id:%d", delicious.ID)
	rwMutex := new(sync.RWMutex)
	rwMutex.Lock()
	defer rwMutex.Unlock()

	marshal, jsonError := json.Marshal(delicious)
	if jsonError != nil {
		log.Printf("json转换错误，错误信息：%v", jsonError)
		return jsonError
	}

	writeLen, writeFileErr := fileWrite.WriteString(string(marshal) + "\n")
	log.Println("写出大小:" + strconv.Itoa(writeLen))
	if writeFileErr != nil {
		log.Printf("写文件错误，错误信息：%v", writeFileErr)
		return writeFileErr
	}

	flushErr := fileWrite.Flush()
	if flushErr != nil {
		log.Fatalf("文件刷新错误，错误信息%v", flushErr)
		return flushErr
	}

	return nil
}

// 错误解析器 如果错误打印500则程序停止运行
func errorHandle(errChan chan error, mutex *sync.RWMutex) {
	for range errChan {
		if errorCount <= 500 {
			fmt.Println(errorCount)
			mutex.Lock()
			errorCount++
			mutex.Unlock()
		} else {
			panic("错误过多，停止运行.")
		}
	}
}

// 处理特殊的空格和空白字符
func replaceAllSpaceAndNewLine(content string) string {
	return strings.ReplaceAll(strings.ReplaceAll(content, "\t", ""), "\n", "")
}

func main() {
	// 初始化一系列对象
	Init()
	start()
}
