package main

import (
	"fmt"
	"io"
	"net/http"
)

func main() {
	client := http.Client{}
	res, err := client.Get("https://cars.mogo.co.ke/auto/8367/nissan-x-trail-2003")
	if err != nil {
		fmt.Println(err)
	}
	if res.StatusCode != 200 {
		fmt.Println("None 200 status code", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%s", body)
}
