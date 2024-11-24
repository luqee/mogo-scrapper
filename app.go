package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	"golang.org/x/net/html"
)

type Car struct {
	Id           int64
	CarId        uint64
	Make         string
	Model        string
	EngineCap    float64
	Transmission string
	FuelType     string
	Year         uint64
	Milage       uint64
	Plate        string
	BodyType     string
	Price        uint64
	Seen         uint64
	Sold         bool
}

func fetchUrl(url string) (*html.Node, error) {
	client := http.Client{}
	res, err := client.Get(url)
	if err != nil {
		fmt.Println(err)
	}
	if res.StatusCode != 200 {
		fmt.Println("None 200 status code", res.StatusCode)
		return nil, errors.New("none 200 status received")
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

func extractId(path string) uint64 {
	re := regexp.MustCompile(`/auto/(\d+)/`)
	matches := re.FindStringSubmatch(path)
	if len(matches) < 2 {
		fmt.Println("no id found in the path")
	}
	id, err := strconv.Atoi(matches[1])
	if err != nil {
		fmt.Println("error converting")
	}
	return uint64(id)
}

func extractCarUrls(host string, root *html.Node, carUrls map[uint64]string) {
	var dfs func(*html.Node)
	dfs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, atr := range n.Attr {
				if atr.Key == "href" && strings.HasPrefix(atr.Val, "/auto/") {
					carUrls[extractId(atr.Val)] = host + atr.Val
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
}

func extractDetails(root *html.Node, car *Car) {
	var details []string
	var dfs func(*html.Node)
	dfs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, atr := range n.Attr {
				if atr.Key == "class" && atr.Val == "font-bold" {
					// get attributes
					fmt.Println(n.FirstChild.Data)
					details = append(details, n.FirstChild.Data)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
	year, _ := strconv.Atoi(details[0])
	car.Year = uint64(year)
	car.BodyType = details[1]
	car.Transmission = details[2]
	car.FuelType = details[3]
	re := regexp.MustCompile(`\d+(\.\d+)?`)
	match := re.FindString(details[4])
	cap, _ := strconv.ParseFloat(match, 64)
	car.EngineCap = cap
	re = regexp.MustCompile(`\d+`)
	matches := re.FindAllString(details[5], -1)
	miles, _ := strconv.Atoi(strings.Join(matches, ""))
	car.Milage = uint64(miles)
	car.Plate = details[6]
}

func getAuctionList(host string) (map[uint64]string, error) {
	var pageNum = 1
	fetchedUrls := make(map[uint64]string, 0)
	for {
		res, err := fetchUrl(host + "/auction?page=" + strconv.Itoa(pageNum))
		if err != nil {
			fmt.Println(err)
			break
		}
		extractCarUrls(host, res, fetchedUrls)
		pageNum++
	}
	return fetchedUrls, nil
}

func getLocalCars(dbConn *pgx.Conn) ([]Car, error) {
	var cars []Car
	rows, err := dbConn.Query(context.Background(), "SELECT * FROM cars WHERE sold=?", 0)
	if err != nil {
		log.Printf("Error while fetching from db: %v", err)
		os.Exit(110)
	}
	rows.Close()
	for rows.Next() {
		var car Car
		if err := rows.Scan(&car.Id, &car.Make, &car.Model, &car.Price); err != nil {
			log.Printf("Error while scanning results: %v", err)
			os.Exit(111)
		}
		cars = append(cars, car)
	}
	if err := rows.Err(); err != nil {
		log.Printf("Error rows: %v", err)
		os.Exit(111)
	}
	return cars, nil
}

func saveCar(car *Car) (int64, error) {
	var id int64
	query := "INSERT INTO cars (carId, make, model, engineCap, transmission, fuelType, year, milage, plate, bodyType, price) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id"
	err := connObj.QueryRow(
		context.Background(),
		query,
		car.CarId, car.Make, car.Model, car.EngineCap, car.Transmission, car.FuelType, car.Year, car.Milage, car.Plate, car.BodyType, car.Price).Scan(&id)
	if err != nil {
		log.Printf("Error saving car")
		return 0, fmt.Errorf("Error saving %v", err)
	}

	return id, nil
}

func getCarsDeets(carsList map[uint64]string) error {
	var car Car
	for carId, carUrl := range carsList {
		res, err := fetchUrl(carUrl)
		if err != nil {
			fmt.Println(err)
			continue
		}
		car = Car{CarId: carId}
		aboutSection := findNode(res, "div", SearchAttr{Key: "class", Value: "vehicle-about__details"})
		extractDetails(aboutSection, &car)
		car.save()
	}
	return nil
}

func filterCars(aucList map[uint64]string, localCars []Car) {
	for _, car := range localCars {
		_, ok := aucList[car.CarId]
		if !ok {
			fmt.Println("Local Car not in list")
			//update db car sold
		} else {
			//update seen
			//remove from list
			delete(aucList, car.CarId)
		}
	}
}

func crawlMogo(dbConn *pgx.Conn) {
	var host string = "https://cars.mogo.co.ke"
	aucList, err := getAuctionList(host)
	if err != nil {
		fmt.Print(err.Error())
	}
	// get cars in storage
	localCars, err := getLocalCars(dbConn)
	if err != nil {
		fmt.Print(err.Error())
	}
	filterCars(aucList, localCars)
	getCarsDeets(aucList)

}

var connObj *pgx.Conn

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// dbConn := connect()
	// crawlMogo(dbConn)
	res, err := fetchUrl("https://cars.mogo.co.ke/auto/10771/nissan-caravan-2008")
	if err != nil {
		fmt.Println(err)
	}
	var car Car
	aboutSection := findNode(res, "div", SearchAttr{Key: "class", Value: "vehicle-about__details"})
	extractDetails(aboutSection, &car)
	fmt.Printf("%v", car)
	// aucList, err := getCarsList(dbConn)
	// if err != nil {
	// 	fmt.Print(err.Error())conn
	// }
	// getCarsDeets(aucList)
}

func connect() *pgx.Conn {
	connObj, err := pgx.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer connObj.Close(context.Background())

	pingErr := connObj.Ping(context.Background())
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	fmt.Println("connected!")
	return connObj
}
