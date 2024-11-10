package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
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
		return nil, errors.New("Not found")
	}
	defer res.Body.Close()
	parsedDoc, err := html.Parse(res.Body)
	if err != nil {
		fmt.Println(err)
	}
	return parsedDoc, nil
}

type SearchAttr struct {
	Key   string
	Value string
}

func findNode(root *html.Node, elem string, attrib SearchAttr) *html.Node {
	// fmt.Println(attrib.Value)
	if root.Type == html.ElementNode && root.Data == elem {
		for _, atr := range root.Attr {
			if atr.Key == attrib.Key && strings.HasPrefix(atr.Val, attrib.Value) {
				return root
			}
		}
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		foundNode := findNode(c, elem, attrib)
		if foundNode != nil {
			return foundNode
		}
	}
	return nil
}

func extractCarUrls(root *html.Node, carUrls []string) {
	var host = "https://cars.mogo.co.ke"
	var dfs func(*html.Node)
	dfs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, atr := range n.Attr {
				if atr.Key == "href" && strings.HasPrefix(atr.Val, "/auto/") {
					carUrls = append(carUrls, host+atr.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
}

func extractDetails(root *html.Node) {

	var dfs func(*html.Node)
	dfs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, atr := range n.Attr {
				if atr.Key == "class" && atr.Val == "font-bold" {
					// get attributes
					fmt.Println(n.FirstChild.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
}

func getCarsList(dbConn *pgx.Conn) ([]string, error) {
	var pageNum = 1
	carUrls := make([]string, 0)
	for {
		res, err := fetchUrl("https://cars.mogo.co.ke/auction?page=" + strconv.Itoa(pageNum))
		if err != nil {
			fmt.Println(err)
			break
		}
		extractCarUrls(res, carUrls)
		pageNum++
	}
	return carUrls, nil
}

func getCarsDeets(carsList []string) error {
	for _, carUrl := range carsList {
		res, err := fetchUrl(carUrl)
		if err != nil {
			fmt.Println(err)
			continue
		}
		aboutSection := findNode(res, "div", SearchAttr{Key: "class", Value: "vehicle-about__details"})
		extractDetails(aboutSection)
	}
	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	dbConn := connect()

	// res, err := fetchUrl("https://cars.mogo.co.ke/auto/10514/toyota-sienta-2008")
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// aboutSection := findNode(res, "div", SearchAttr{Key: "class", Value: "vehicle-about__details"})
	// extractDetails(aboutSection)
	// aucList, err := getCarsList(dbConn)
	// if err != nil {
	// 	fmt.Print(err.Error())
	// }
	// getCarsDeets(aucList)
}

func connect() *pgx.Conn {
	conn, err := pgx.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())

	pingErr := conn.Ping(context.Background())
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	fmt.Println("Connected!")
	return conn
}
