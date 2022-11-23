package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/djimenez/iconv-go"
	"github.com/go-resty/resty/v2"
)

func main() {

	client := resty.New()

	client.SetBaseURL("https://www.meishichina.com/")
	get, err := client.R().EnableTrace().Get("")

	if err != nil {
		fmt.Println(err)

		return
	}

	utfBody, err := iconv.NewReader(res.Body, charset, "utf-8")

	doc, err := goquery.NewDocumentFromReader(utfBody)

	doc.Find(".on li a").Each(func(i int, selection *goquery.Selection) {
		content := selection.Find("p").Text()
		fmt.Println(content)
	})

	//policy := bluemonday.UGCPolicy()
	//
	//sanitize := policy.Sanitize(`<a onblur="alert(secret)" href="http://www.google.com">Google</a>`)
	//
	//println(sanitize)

	//fmt.Println(get)
}
