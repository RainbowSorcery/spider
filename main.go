package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"github.com/PuerkitoBio/goquery"
	"github.com/go-resty/resty/v2"
	"github.com/rs/zerolog/log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

/*
*'
菜谱分类struct
categoryName: 分类名称
categoryUrl: 分类URL
*/
type Category struct {
	categoryName string
	categoryUrl  string
}

type Food struct {
	Id            string `json:"id"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Image         string `json:"image"`
	Flavor        string `json:"flavor"`
	Craft         string `json:"craft"`
	TimeConsuming string `json:"time_consuming"`
	Difficulty    string `json:"difficulty"`
	Ware          string `json:"ware"`

	Category  []string       `json:"category"`
	Materials []FoodMaterial `json:"materials"`
	Steps     []FoodStep     `json:"steps"`
}

type proxyResponseStruct struct {
	Schema    string `json:"schema"`
	Proxy     string `json:"proxy"`
	Source    string `json:"source"`
	CheckTime string `json:"check_time"`
}

type FoodMaterial struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
	Type   string `json:"type"`
}

type FoodStep struct {
	Image   string `json:"image"`
	Content string `json:"content"`
}

// 爬取url地址
const baseUrl = "https://home.meishichina.com/"

// 代理池
const proxyUrl = "http://150.158.87.243:8080/proxy/get"

/*
*
获取分类页面
*/
func spiderCategoryPage(wg *sync.WaitGroup, channel chan []byte, client *resty.Client) {
	defer wg.Done()

	functionStart := time.Now().Unix()

	log.Info().Msg("开始爬取recipe-type.html页")
	subUrl := "/recipe-type.html"
	response, err := client.R().EnableTrace().Get(baseUrl + subUrl)
	if err != nil {
		log.Err(err).
			Str("response", response.String()).
			Msg(baseUrl + subUrl + "页面爬取错误!")
	}

	if response.StatusCode() == 200 {
		channel <- response.Body()
	} else {
		log.Warn().
			Int("statusCode", response.StatusCode()).
			Str("functionName", "spiderCategoryPage(wg *sync.WaitGroup, channel chan []byte, client *resty.Client)").
			Str("url", client.BaseURL+subUrl).
			Str("response", response.String()).
			Msg("页面爬取错误!")
	}
	close(channel)
	functionEnd := time.Now().Unix()
	log.Info().Int64("spiderCategoryPage执行时间", functionEnd-functionStart).Msg("")
}

/*
*	分析页面内容
 */
func parseCategoryPage(wg *sync.WaitGroup, categoryPageChannel chan []byte, categoryChannel chan Category) {
	defer wg.Done()
	log.Info().Msg("开始执行parseCategoryPage")

	functionStart := time.Now().Unix()

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

		if categoryURLExists && categoryName != "" {
			categoryChannel <- Category{
				categoryName: categoryName,
				categoryUrl:  categoryURL,
			}
		} else {
			log.Warn().
				Msg("超链接URL元素不存在")
		}
	})

	close(categoryChannel)
	functionEnd := time.Now().Unix()
	log.Info().Int64("parseCategoryPage执行时间", functionEnd-functionStart).Msg("")
}

func spiderCategoryFood(wg *sync.WaitGroup, categoryChannel chan Category, client *resty.Client, channel chan string, detailChannel chan []byte) {
	defer wg.Done()
	functionStart := time.Now().Unix()
	log.Info().Msg("开始执行spiderCategoryFood")

	for category := range categoryChannel {
		response, err := client.R().Get(category.categoryUrl)
		if err != nil {
			log.Err(err).
				Str("functionName", "spiderCategoryFood(wg *sync.WaitGroup, categoryChannel chan category, client *resty.Client)").
				Str("parseContent", response.String()).
				Msg("页面解析失败!")
		}

		if response.StatusCode() != 200 {
			log.Warn().
				Str("url", category.categoryUrl).
				Msg("状态码错误")
		}

		recursiveGetPage(category.categoryUrl, client, wg, channel, detailChannel)
		functionEnd := time.Now().Unix()
		log.Info().Int64("spiderCategoryFood执行时间", functionEnd-functionStart).Msg("")
	}
}

/*
*
递归调用获取每个分类的分页内容
*/
func recursiveGetPage(url string, client *resty.Client, wg *sync.WaitGroup, channel chan string, detailChannel chan []byte) {
	response, err := client.R().Get(url)

	log.Info().Msg("开始执行recursiveGetPage")

	functionStart := time.Now().Unix()

	if err != nil {
		log.Err(err)
	}

	reader := bytes.NewReader(response.Body())
	fromReader, err := goquery.NewDocumentFromReader(reader)

	if err != nil {
		log.Err(err)
	}
	nextPageUrl, exists := fromReader.
		Find("a:contains(下一页)").
		Attr("href")

	log.Debug().Str("nextPage", nextPageUrl).Msg("")

	if exists {

		if err != nil {
			log.Err(err)
		}

		wg.Add(1)
		go parsingPageFood(client, wg, url, detailChannel)

		functionEnd := time.Now().Unix()
		log.Info().Int64("recursiveGetPage执行时间", functionEnd-functionStart).Msg("")

		recursiveGetPage(nextPageUrl, client, wg, channel, detailChannel)
	}
}

/*
*
解析分页
*/
func parsingPageFood(client *resty.Client, wg *sync.WaitGroup, url string, detailChannel chan []byte) {
	defer wg.Done()
	functionStart := time.Now().Unix()
	log.Info().Msg("开始执行parsingPageFood")

	get, err2 := client.R().Get(url)
	if err2 != nil {
		log.Err(err2)
	}
	reader := bytes.NewReader(get.Body())
	fromReader, err := goquery.NewDocumentFromReader(reader)

	if err != nil {
		log.Err(err)
	}

	fromReader.Find("#J_list > ul").Each(func(i int, selection *goquery.Selection) {

		// 详情页
		foodDetailUrl, foodDetailUrlExists := selection.Find("li div.detail > h2 > a").Attr("href")
		if foodDetailUrlExists {
			foodDetailResponse, foodDetailError := client.R().Get(foodDetailUrl)
			if foodDetailError != nil {
				log.Err(foodDetailError)
			}

			if foodDetailResponse.StatusCode() != 200 {
				time.Sleep(time.Second)
				log.Warn().Int("code", foodDetailResponse.StatusCode()).Str("url", foodDetailUrl).Msg("状态码异常")
			} else {
				log.Debug().Str("url", url).Msg("parsingPageFood(client *resty.Client, wg *sync.WaitGroup, channel chan []byte, detailChannel chan []byte)")

				detailChannel <- foodDetailResponse.Body()
			}

		}
	})
	functionEnd := time.Now().Unix()
	log.Info().Int64("parsingPageFood执行时间", functionEnd-functionStart).Msg("")
}

/*
*
组装food struct
*/
func parseFoodDetail(detailChannel chan []byte, wg *sync.WaitGroup, channel chan Food) {
	defer wg.Done()
	log.Info().Msg("开始执行parseFoodDetail")

	functionStart := time.Now().Unix()

	for detail := range detailChannel {
		food := Food{}

		reader := bytes.NewReader(detail)

		document, err := goquery.NewDocumentFromReader(reader)

		if err != nil {
			log.Err(err)
		}

		id, idExists := document.Find("#recipe_id").Attr("value")
		title := document.Find("#recipe_title").Text()
		img, imgExists := document.Find("#recipe_De_imgBox > a > img").Attr("src")
		materials := make([]FoodMaterial, 0)
		document.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > fieldset").
			Each(func(i int, selection *goquery.Selection) {
				Type := selection.Find("legend").Text()
				name := selection.Find("li > span.category_s1 > a > b").Text()
				amount := selection.Find("span.category_s2").Text()

				material := FoodMaterial{
					Name:   name,
					Amount: amount,
					Type:   Type,
				}
				materials = append(materials, material)
			})
		food.Materials = materials

		description := document.Find("#block_txt1").Text()
		compile, err := regexp.Compile("\\s*|t|r|n")
		if err != nil {
			log.Err(err)
		}

		food.Description = compile.ReplaceAllString(description, "")

		elements := make([]string, 0)
		document.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > div.recipeCategory_sub_R.mt30.clear > ul li").
			Each(func(i int, selection *goquery.Selection) {
				name, titleExits := selection.Find("span > a").Attr("title")
				if titleExits {
					elements = append(elements, name)
				}
			})

		if len(elements) == 4 {
			food.Flavor = elements[0]
			food.Craft = elements[1]
			food.TimeConsuming = elements[2]
			food.Difficulty = elements[3]
		}

		text := document.Find(".recipeTip").Text()
		text = strings.ReplaceAll(text, "\t", "")
		split := strings.Split(text, "\n")

		flag := false
		categoryList := make([]string, 0)
		for i := range split {
			if strings.HasPrefix(split[i], "使用的厨具：") {
				food.Ware = strings.Split(split[i], "：")[1]
			}

			if flag && i < len(split)-1 {
				categoryList = append(categoryList, split[i])
			}

			if split[i] == "所属分类：" {
				flag = true
			}

		}
		food.Category = categoryList

		steps := make([]FoodStep, 0)
		document.Find("body > div.wrap > div > div.space_left > div.space_box_home > div > div.recipeStep > ul > li").
			Each(func(i int, selection *goquery.Selection) {
				step := FoodStep{
					Image:   "",
					Content: "",
				}
				content := selection.Find("div.recipeStep_word").Text()
				compile, err2 := regexp.Compile("^[0-9]*")
				if err2 != nil {
					log.Err(err2)
				}
				content = compile.ReplaceAllString(content, "")

				img, imgExits := selection.Find(" div.recipeStep_img > img").Attr("src")
				if imgExits {
					step.Image = img
					step.Content = content
				}
				steps = append(steps, step)
			})

		food.Steps = steps

		food.Title = title
		if idExists {
			food.Id = id
		}

		if imgExists {
			food.Image = img
		}

		channel <- food
	}

	close(channel)

	functionEnd := time.Now().Unix()
	log.Info().Int64("parseFoodDetail执行时间", functionEnd-functionStart).Msg("")
}

/*
*
写文件
*/
func writeFile(foodChannel chan Food, group *sync.WaitGroup, filePath string) {
	defer group.Done()

	filePath = filePath
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		log.Err(err)
	}
	writer := bufio.NewWriter(file)

	defer func(file *os.File) {
		err := file.Close()
		if err != nil {
			log.Err(err)
		}
	}(file)

	flushError := writer.Flush()
	if flushError != nil {
		log.Err(flushError)
		return
	}
	for food := range foodChannel {
		foodToJson, err := json.Marshal(food)

		if err != nil {
			log.Err(err)
			return
		}
		// 替换特殊字符
		stringJson := string(foodToJson)

		i, err := writer.WriteString(stringJson + "\n")
		err = writer.Flush()
		if err != nil {
			log.Err(err)
			return
		}
		if err != nil {
			log.Err(err)
			return
		}
		log.Info().Int("i", i)
	}
}

func main() {
	httpClient := resty.New()
	// 十次重试 每次间隔一秒
	httpClient.SetRetryCount(10)
	httpClient.SetRetryWaitTime(time.Second)
	httpClient.SetProxy("http://127.0.0.1:7890")
	httpClient.AddRetryCondition(func(response *resty.Response, err error) bool {
		if response.StatusCode() != 200 {
			return true
		}
		return false
	})

	// 分类页面爬取channel 如果channel中有数据确没协程拿则会使程序阻塞
	categoryPageChannel := make(chan []byte)
	// 分类信息channel
	categoryChannel := make(chan Category)
	// 每页分类channel
	categoryFoodChannel := make(chan string)
	// 详情url channel
	foodDetailChannel := make(chan []byte)
	// 写文件需要获取整个food struct的channel
	writeFileChannel := make(chan Food)

	var wg sync.WaitGroup

	wg.Add(1)
	go spiderCategoryPage(&wg, categoryPageChannel, httpClient)
	wg.Add(1)
	go parseCategoryPage(&wg, categoryPageChannel, categoryChannel)

	// 开启20个协程遍历分类url
	wg.Add(1)
	for i := 0; i < 20; i++ {
		go spiderCategoryFood(&wg, categoryChannel, httpClient, categoryFoodChannel, foodDetailChannel)
	}

	wg.Add(1)
	go parseFoodDetail(foodDetailChannel, &wg, writeFileChannel)

	wg.Add(1)
	// 开20个协程写文件
	for i := 0; i < 20; i++ {
		go writeFile(writeFileChannel, &wg, "food.json")
	}

	wg.Wait()
}
