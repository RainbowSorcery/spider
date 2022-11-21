package main

import (
	"github.com/go-resty/resty/v2"
	"github.com/microcosm-cc/bluemonday"
)

func main() {
	client := resty.New()

	client.SetBaseURL("https://www.meishichina.com/")
	//get, err := client.R().EnableTrace().Get("")

	//if err != nil {
	//	fmt.Println(err)
	//
	//	return
	//}

	policy := bluemonday.UGCPolicy()

	sanitize := policy.Sanitize(`<a onblur="alert(secret)" href="http://www.google.com">Google</a>`)

	println(sanitize)

	//fmt.Println(get)
}
