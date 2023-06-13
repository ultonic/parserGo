package main

import (
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "myapp",
	Short: "My Application",
	Long:  "A command-line application",
	Run: func(cmd *cobra.Command, args []string) {
		handleDocuments()

	},
}

var secondCmd = &cobra.Command{
	Use:   "second",
	Short: "Second Command",
	Long:  "A second command",
	Run: func(cmd *cobra.Command, args []string) {
		doEnrichment()
	},
}

type DetailedContract struct {
	Content struct {
		StopReason string `json:"stopReason"`
		Comment    string `json:"text"`
	} `json:"content"`
}

type Document struct {
	MainInfo          string            `json:"mainInfo"`
	Number            string            `json:"number"`
	GUID              string            `json:"guid"`
	PublishDate       string            `json:"publishDate"`
	IsAnnuled         bool              `json:"isAnnuled"`
	Type              string            `json:"type"`
	BodyHighlights    []string          `json:"bodyHighlights"`
	DocumentsWithHits []DocumentWithHit `json:"documentsWithHits"`
}

type Contract struct {
	ID          int             `json:"id"`
	Guid        string          `json:"guid"`
	Type        string          `json:"type"`
	Date        time.Time       `json:"publishDate"`
	Number      string          `json:"number"`
	Contract    string          `json:"contract"`
	Lessor      string          `json:"lessor"`
	Lessee      string          `json:"lessee"`
	OGRN        string          `json:"ogrn"`
	INN         string          `json:"inn"`
	StopReason  string          `json:"stop_reason"`
	UserComment string          `json:"user_comment"`
	ListItemRaw json.RawMessage `json:"list_item_raw"`
	ItemRaw     json.RawMessage `json:"item_raw"`
	CreatedAt   time.Time       `json:"-"`
	UpdatedAt   time.Time       `json:"-"`
	Enriched    bool            `json:"enriched"`
}

type DocumentWithHit struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
}

type PageData struct {
	Documents []Document `json:"pageData"`
	Found     int        `json:"found"`
}

func main() {
	rootCmd.AddCommand(secondCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func doEnrichment() {
	db, err := sql.Open("mysql", "root:Ppu5V2Jfor@tcp(127.0.0.1:3306)/parser")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query("SELECT guid FROM contract WHERE enriched = false")
	if err != nil {
		log.Fatal(err)
	}

	for rows.Next() {
		// Define variables to hold the column values
		var guid string

		// Scan the column values into variables
		err := rows.Scan(&guid)
		if err != nil {
			log.Fatal(err)
		}

		time.Sleep(3 * time.Second)

		body, err := requestEnrichmentData(guid)
		if err != nil {
			log.Fatal(err)
		}

		if body == nil {
			continue
		}

		var response DetailedContract
		err = json.Unmarshal(body, &response)
		if err != nil {
			log.Fatal(err)
		}

		// fmt.Printf("Type of Comment: %T\n", response.Content.Comment)
		// fmt.Printf("Type of StopReason: %T\n", response.Content.StopReason)

		if response.Content.Comment != "" || response.Content.StopReason != "" {
			stmt, err := db.Prepare("UPDATE contract SET user_comment = ?, stop_reason = ?, enriched = ?, item_raw = ? WHERE guid = ?")
			if err != nil {
				log.Fatal(err)
			}
			defer stmt.Close()

			content, _ := json.Marshal(response.Content)

			// Execute the SQL statement with the provided values
			_, err = stmt.Exec(response.Content.Comment, response.Content.StopReason, true, content, guid)
			if err != nil {
				log.Fatal(err)
			}

			fmt.Println("Data updated successfully!")
		}
	}
}

func handleDocuments() {
	db, err := sql.Open("mysql", "root:Ppu5V2Jfor@tcp(127.0.0.1:3306)/parser")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Query the latest date from the contract table
	var latestDateStr sql.NullString
	err = db.QueryRow("SELECT DATE_FORMAT(MAX(date), '%Y-%m-%d %H:%i:%s') FROM contract").Scan(&latestDateStr)
	if err != nil && err != sql.ErrNoRows {
		log.Fatal(err)
	}

	var date time.Time
	if latestDateStr.Valid {
		date, err = time.Parse("2006-01-02 15:04:05", latestDateStr.String)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		date = time.Now().UTC().Truncate(24 * time.Hour)
	}

	// Step 1: Request list of documents
	pageData, err := requestDocuments(date.Format("2006-01-02T15:04:05.999"))
	if err != nil {
		log.Fatalf("Failed to perform operation: %s", err)
	}

	writeIntoDB(pageData.Documents)
}

func writeIntoDB(documents []Document) error {
	// Open a connection to the MySQL database
	db, err := sql.Open("mysql", "root:Ppu5V2Jfor@tcp(127.0.0.1:3306)/parser")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	for _, doc := range documents {
		if doc.Type == "Заключение договора финансовой аренды (лизинга)" {
			continue
		}
		var contract Contract
		contract.Type = doc.Type
		layout := "2006-01-02T15:04:05"

		pubDate := doc.PublishDate

		index := strings.LastIndex(doc.PublishDate, ".")
		if index != -1 {
			pubDate = doc.PublishDate[:index]

		}

		contract.Date, _ = time.Parse(layout, pubDate)
		contract.Number = doc.Number
		contract.Guid = doc.GUID
		contract.Contract = extractContractInfo(doc.MainInfo)
		contract.Lessor = extractLessorInfo(doc.MainInfo)
		contract.Lessee = extractLesseeInfo(doc.MainInfo)
		contract.OGRN = extractOgrnInfo(doc.MainInfo)
		contract.INN = extractInnInfo(doc.MainInfo)
		contract.ListItemRaw, _ = json.Marshal(doc)
		contract.ItemRaw = json.RawMessage(`{}`)

		// Prepare the SQL statement for inserting data into the contract table
		stmt, err := db.Prepare(`INSERT INTO contract (type, date, number, contract, lessor, lessee, ogrn, inn, stop_reason, user_comment, list_item_raw, item_raw, guid) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		if err != nil {
			log.Fatal(err)
		}
		defer stmt.Close()

		// Execute the SQL statement to insert the contract data into the database
		_, err = stmt.Exec(contract.Type, contract.Date, contract.Number, contract.Contract, contract.Lessor, contract.Lessee, contract.OGRN, contract.INN, contract.StopReason, contract.UserComment, contract.ListItemRaw, contract.ItemRaw, contract.Guid)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("Data inserted successfully!")
	}

	return nil
}

func extractContractInfo(mainInfo string) string {
	re := regexp.MustCompile(`Договор:\s(.+)\r\n`)
	match := re.FindStringSubmatch(mainInfo)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

// Function to extract the lessor information from the main info
func extractLessorInfo(mainInfo string) string {
	re := regexp.MustCompile(`Лизингодатель:\s(.+?)(?:,\sОГРН|\r\n|Лизингополучатель)`)
	match := re.FindStringSubmatch(mainInfo)
	if len(match) > 1 {
		lessorInfo := match[1]
		lessorInfo = strings.TrimPrefix(lessorInfo, "ООО ")
		return lessorInfo
	}
	return ""
}

func extractLesseeInfo(mainInfo string) string {
	re := regexp.MustCompile(`Лизингополучатель:\s([^,]+)`)
	match := re.FindStringSubmatch(mainInfo)
	if len(match) > 1 {
		lessorInfo := match[1]
		lessorInfo = strings.TrimPrefix(lessorInfo, "ООО ")
		return lessorInfo
	}
	return ""
}

func extractOgrnInfo(mainInfo string) string {
	re := regexp.MustCompile(`ОГРН:\s(\d+)`)
	match := re.FindStringSubmatch(mainInfo)
	if len(match) > 1 {
		lessorInfo := match[1]
		lessorInfo = strings.TrimPrefix(lessorInfo, "ООО ")
		return lessorInfo
	}
	return ""
}

func extractInnInfo(mainInfo string) string {
	re := regexp.MustCompile(`ИНН:\s(\d+)`)
	match := re.FindStringSubmatch(mainInfo)
	if len(match) > 1 {
		lessorInfo := match[1]
		lessorInfo = strings.TrimPrefix(lessorInfo, "ООО ")
		return lessorInfo
	}
	return ""
}

func getDecimalFormat(dateString string) string {
	decimalPart := ""
	if dotIndex := findDotIndex(dateString); dotIndex != -1 {
		decimalPart = dateString[dotIndex+1:]
	}

	if decimalPart == "" {
		return "000"
	}

	// Pad with zeros to ensure a fixed length of 3 digits
	decimalPart = decimalPart + strings.Repeat("0", 3-len(decimalPart))

	return decimalPart
}

func findDotIndex(dateString string) int {
	for i, char := range dateString {
		if char == '.' {
			return i
		}
	}
	return -1
}

func convertToDayRange(date string) (string, string) {
	layout := "2006-01-02T15:04:05"
	parsedTime, err := time.Parse(layout, date)
	if err != nil {
		log.Fatal(err)
	}

	start := parsedTime.Format("2006-01-02T00:00:00.000")
	end := parsedTime.Format("2006-01-02T23:59:59.999")

	return start, end
}

func requestDocuments(date string) (*PageData, error) {
	start, end := convertToDayRange(date)
	targetUrl := fmt.Sprintf("https://fedresurs.ru/backend/encumbrances?offset=0&limit=10000&searchString=%s&group=Leasing&publishDateStart=%s&publishDateEnd=%s", "%D0%B4%D0%BE%D0%B3%D0%BE%D0%B2%D0%BE%D1%80", start, end)

	client := &http.Client{}
	req, err := http.NewRequest("GET", targetUrl, nil)
	if err != nil {
		log.Fatal(err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/97.0.4692.99 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Referer", fmt.Sprintf("https://fedresurs.ru/search/encumbrances?offset=0&limit=10000&searchString=%s&group=Leasing&publishDateStart=%s&publishDateEnd=%s", "%D0%B4%D0%BE%D0%B3%D0%BE%D0%B2%D0%BE%D1%80", start, end))

	// Introduce a delay before sending the request
	time.Sleep(5 * time.Second)

	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	if len(body) == 0 {
		return nil, fmt.Errorf("empty response body")
	}

	var pageData PageData
	err = json.Unmarshal(body, &pageData)
	if err != nil {
		return nil, err
	}

	return &pageData, nil
}

func requestEnrichmentData(guid string) ([]byte, error) {
	url := fmt.Sprintf("https://fedresurs.ru/backend/sfactmessages/%s", guid)

	// Create a new HTTP request with the desired URL and method
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}

	// Set the request headers
	// req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Accept-Language", "en-GB,en-US;q=0.9,en;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Host", "fedresurs.ru")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/16.4 Safari/605.1.15")
	req.Header.Set("Referer", fmt.Sprintf("https://fedresurs.ru/sfactmessage/%s", guid))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cookie", "_ym_visorc=b; qrator_msid=1685824960.242.0Q4r6L5yvceGIiVZ-6io13rif923gscg667ju2orrshj9rokg; _ym_d=1685808284; _ym_isad=2; _ym_uid=1685808284248808853")
	req.Header.Set("Sec-Fetch-Dest", "empty")

	// Create an HTTP client
	client := http.Client{}

	// Send the request
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}

	// Close the response body when done reading
	defer resp.Body.Close()

	// Print the response status code
	fmt.Println("Response status code:", resp.StatusCode)

	if resp.StatusCode != 200 {
		return nil, nil
	}

	if resp.Header.Get("Content-Encoding") == "gzip" {
		fmt.Println("It's gzip")
		// Create a GZIP reader to decompress the response body
		reader, err := gzip.NewReader(resp.Body)
		if err != nil {
			log.Fatal(err)
		}
		defer reader.Close()

		body, err := ioutil.ReadAll(reader)
		if err != nil {
			log.Fatal(err)
		}

		return body, nil
	} else {
		fmt.Println("It's not gzip")
		// Read the response body directly
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		return body, nil
	}
}
