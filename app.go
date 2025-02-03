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
	Description  string
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

func extractDetails(root *html.Node, car *Car) error {
	var details []string
	// var descr string
	var dfs func(*html.Node)
	dfs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, atr := range n.Attr {
				if atr.Key == "class" && atr.Val == "font-bold" {
					// get attributes
					details = append(details, n.FirstChild.Data)
				}
				if atr.Key == "class" && atr.Val == "[&_p]:text-body [&_p]:text-medium-dark [&_p]:overflow-hidden [&_p]:max-h-[366px] md:[&_p]:max-h-[264px] lg:[&_p]:max-h-[240px] my-6 md:my-8 lg:my-6" {
					// get attributes
					car.Description = n.FirstChild.FirstChild.Data
					makePattern := regexp.MustCompile(`(?i)Make\s*-\s*(.+)\s*Model`)
					modelPattern := regexp.MustCompile(`(?i)Model\s*-\s*(.+)\s*Manuf`)
					makeMatch := makePattern.FindStringSubmatch(car.Description)
					modelMatch := modelPattern.FindStringSubmatch(car.Description)
					if len(makeMatch) > 1 {
						car.Make = makeMatch[1]
					} else {
						fmt.Println("Make not found")
					}

					if len(modelMatch) > 1 {
						car.Model = modelMatch[1]
					} else {
						fmt.Println("Model not found")
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
	fmt.Printf("found these details for car %d \n%v\n", car.CarId, details)
	if len(details) == 7 {
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
		return nil
	}
	return errors.New("Insufficient fields")
}

func extractPrice(root *html.Node, car *Car) {
	var dfs func(*html.Node)
	dfs = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, atr := range n.Attr {
				if atr.Key == "class" && atr.Val == "text-high-dark text-xl leading-8 font-semibold mt-0.5 order-2" {
					// get attributes
					priceRe := regexp.MustCompile(`\d+`)
					match := priceRe.FindAllString(n.FirstChild.Data, -1)
					price, _ := strconv.Atoi(strings.Join(match, ""))
					car.Price = uint64(price)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			dfs(c)
		}
	}
	dfs(root)
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
		fmt.Printf("Fetched page %d\n", pageNum)
		carsGrid := findNode(res, "div", SearchAttr{Key: "class", Value: "grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-2 xl:grid-cols-3 xl:gap-8"})
		if carsGrid == nil {
			fmt.Printf("Reached past end. page: %d\n", pageNum)
			break
		}
		extractCarUrls(host, res, fetchedUrls)
		pageNum++
	}
	return fetchedUrls, nil
}

func getLocalCars(dbConn *pgx.Conn) ([]Car, error) {
	fmt.Printf("Fetching local cars")
	var cars []Car
	rows, err := dbConn.Query(context.Background(), "SELECT id, car_id, seen FROM mogo WHERE sold=$1", false)
	if err != nil {
		log.Printf("Error while fetching from db: %v", err)
		os.Exit(110)
	}
	defer rows.Close()
	for rows.Next() {
		var car Car
		if err := rows.Scan(&car.Id, &car.CarId, &car.Seen); err != nil {
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

func saveCar(connObj *pgx.Conn, car *Car) (int64, error) {
	var id int64
	query := "INSERT INTO mogo (car_id, make, model, engine_cap, transmission, fuel_type, year, mileage, plate, body_type, price, description) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id"
	err := connObj.QueryRow(
		context.Background(),
		query,
		car.CarId, car.Make, car.Model, car.EngineCap, car.Transmission, car.FuelType, car.Year, car.Milage, car.Plate, car.BodyType, car.Price, car.Description).Scan(&id)
	if err != nil {
		log.Printf("Error saving car %v", err)
		return 0, fmt.Errorf("error saving %v", err)
	}

	return id, nil
}

func updateSold(dbConn *pgx.Conn, car *Car) {
	query := "UPDATE mogo SET sold=true WHERE id=$1"
	cmdTag, err := dbConn.Exec(
		context.Background(),
		query,
		car.Id)
	if err != nil {
		log.Printf("Error update car %v\n", err)
	}
	if cmdTag.RowsAffected() != 1 {
		log.Printf("No row found to update for %v\n", car.CarId)
	} else {
		log.Printf("Updated car %v\n", car.CarId)
	}
}

func updateSeen(dbConn *pgx.Conn, car *Car) {
	query := "UPDATE mogo SET seen=$1 WHERE id=$2"
	cmdTag, err := dbConn.Exec(
		context.Background(),
		query,
		car.Seen+1, car.Id)
	if err != nil {
		log.Printf("Error updating car %v\n", err)
	}
	if cmdTag.RowsAffected() != 1 {
		log.Printf("No row found to update for %v\n", car.CarId)
	} else {
		log.Printf("Updated car %v\n", car.CarId)
	}
}

func getCarsDeets(dbConn *pgx.Conn, carsList map[uint64]string) error {
	var car Car
	for carId, carUrl := range carsList {
		res, err := fetchUrl(carUrl)
		if err != nil {
			fmt.Println(err)
			continue
		}
		car = Car{CarId: carId}
		priceSection := findNode(res, "div", SearchAttr{Key: "class", Value: "ds-vehicle-card-pricings py-2 px-0 ds-vehicle-card-pricings--no-borders cp-vehicle-card-mogo"})
		extractPrice(priceSection, &car)
		aboutSection := findNode(res, "section", SearchAttr{Key: "class", Value: "vehicle-about"})
		err = extractDetails(aboutSection, &car)
		if err != nil {
			log.Printf("%s", err)
		} else {
			saveCar(dbConn, &car)
		}
	}
	return nil
}

func filterCars(dbConn *pgx.Conn, aucList map[uint64]string, localCars []Car) {
	for _, car := range localCars {
		_, ok := aucList[car.CarId]
		if !ok {
			updateSold(dbConn, &car)
			//update db car sold
		} else {
			delete(aucList, car.CarId)
			updateSeen(dbConn, &car)
		}
	}
	fmt.Printf("After filter %v\n", aucList)
}

func main() {

	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	conn := connect()
	defer conn.Close(context.Background())
	createTabe(conn)
	localCars, err := getLocalCars(conn)
	if err != nil {
		fmt.Print(err.Error())
	}
	var host string = "https://cars.mogo.co.ke"
	aucList, err := getAuctionList(host)
	if err != nil {
		fmt.Print(err.Error())
	}
	filterCars(conn, aucList, localCars)
	getCarsDeets(conn, aucList)

	// crawlMogo(dbConn)
	// res, err := fetchUrl("https://cars.mogo.co.ke/auto/10859/land-rover-range-rover-vogue-2005")
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// var car Car
	// aboutSection := findNode(res, "section", SearchAttr{Key: "class", Value: "vehicle-about"})
	// extractDetails(aboutSection, &car)

	// saveCar(conn, &car)
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
	pingErr := connObj.Ping(context.Background())
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	fmt.Println("connected!")
	return connObj

}

func createTabe(connObj *pgx.Conn) {
	query := `CREATE TABLE IF NOT EXISTS mogo (
		id SERIAL PRIMARY KEY,
		car_id int UNIQUE NOT NULL,
		make VARCHAR(100) NOT NULL,
		model VARCHAR(100) NOT NULL,
		year int NOT NULL,
		mileage int NOT NULL,
		transmission VARCHAR(100) NOT NULL,
		engine_cap NUMERIC(3,1) NOT NULL,
		fuel_type VARCHAR(50) NOT NULL,
		plate VARCHAR(50) NOT NULL,
		body_type VARCHAR(50) NOT NULL,
		price int NOT NULL,
		seen int DEFAULT 1,
		sold BOOLEAN DEFAULT false,
		description TEXT NOT NULL,
		created timestamp DEFAULT NOW()
	)`
	_, err := connObj.Exec(context.Background(), query)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to create tabke : %v\n", err)
		os.Exit(1)
	}

}
