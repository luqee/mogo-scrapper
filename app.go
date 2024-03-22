package main

import (
	"fmt"
	"net/http"

	"golang.org/x/net/html"
)

func fetchUrl(url string) (*html.Node, error) {
	client := http.Client{}
	res, err := client.Get(url)
	if err != nil {
		fmt.Println(err)
	}
	if res.StatusCode != 200 {
		fmt.Println("None 200 status code", res.StatusCode)
	}
	defer res.Body.Close()
	parsedDoc, err := html.Parse(res.Body)
	if err != nil {
		fmt.Println(err)
	}
	return parsedDoc, nil
}

func getCarsList() ([]string, error) {
	res, err := fetchUrl("https://cars.mogo.co.ke/auction")
	if err != nil {
		fmt.Println(err)
	}
	fmt.Print(res)
	cars := make([]string, 0)
	cars = append(cars, "https://cars.mogo.co.ke/auto/8367/nissan-x-trail-2003")
	return cars, nil
}

func getCarsDeets(carsList []string) error {
	res, err := fetchUrl(carsList[0])
	if err != nil {
		fmt.Println(err)
	}
	fmt.Print(res)
	return nil
}

func main() {
	aucList, err := getCarsList()
	if err != nil {
		fmt.Print(err.Error())
	}

	getCarsDeets(aucList)
}
