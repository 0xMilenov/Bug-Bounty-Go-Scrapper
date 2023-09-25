package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Bounty struct {
	ID          string   `json:"id"`
	Project     string   `json:"project"`
	UpdatedDate string   `json:"updatedDate"`
	AssetLinks  []string `json:"assetLinks"`
}

type Difference struct {
	ID                 string   `json:"id"`
	Project            string   `json:"project"`
	ExistingUpdatedDate string   `json:"existing_updatedDate"`
	NewUpdatedDate     string   `json:"new_updatedDate"`
	LinkDiff           []string `json:"link_diff"`
}

func fetchSourceCode(url string, user_agent string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", user_agent)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func extractTokenFromSource(source string) string {
	r := regexp.MustCompile(`/_next/static/([^/]+)/_buildManifest\.js`)
	match := r.FindStringSubmatch(source)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func fetchDataUsingToken(token string) ([]Bounty, error) {
	url := fmt.Sprintf("https://immunefi.com/_next/data/%s/explore.json", token)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var jsonData map[string]map[string][]Bounty
	err = json.Unmarshal(body, &jsonData)
	if err != nil {
		return nil, err
	}

	bounties := jsonData["pageProps"]["bounties"]
	return bounties, nil
}

func connectToDatabase(mongo_uri string, mongo_db string) (*mongo.Database, error) {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(mongo_uri))
	if err != nil {
		return nil, err
	}
	db := client.Database(mongo_db)
	return db, nil
}

// Additional functions for MongoDB operations (initializeBountiesTableIfEmpty, compareWithExistingData, etc.) should be added here.

func insertIntoDiffTable(differences []Difference, db *mongo.Database) {
	diffCollection := db.Collection("differences")
	for _, diff := range differences {
		filter := bson.M{"project": diff.Project}
		update := bson.M{
			"$set": bson.M{
				"id":                 diff.ID,
				"existing_updatedDate": diff.ExistingUpdatedDate,
				"new_updatedDate":     diff.NewUpdatedDate,
				"link_diff":           diff.LinkDiff,
			},
		}
		opts := options.Update().SetUpsert(true)
		_, err := diffCollection.UpdateOne(context.TODO(), filter, update, opts)
		if err != nil {
			log.Println("Error updating difference:", err)
		}
	}
}

func compareWithExistingData(newData []Bounty, db *mongo.Database) []Difference {
	bountiesCollection := db.Collection("bounties")
	cursor, err := bountiesCollection.Find(context.TODO(), bson.M{})
	if err != nil {
		log.Println("Error fetching existing data:", err)
		return nil
	}
	defer cursor.Close(context.TODO())

	var existingDataList []Bounty
	if err = cursor.All(context.TODO(), &existingDataList); err != nil {
		log.Println("Error decoding existing data:", err)
		return nil
	}

	existingData := make(map[string]Bounty)
	for _, item := range existingDataList {
		existingData[item.Project] = item
	}

	var differences []Difference

	for _, item := range newData {
		assetLinks := fetchAssetLinksForBounty(item.ID)
		existingItem, exists := existingData[item.Project]

		if exists {
			if existingItem.UpdatedDate != item.UpdatedDate {
				fmt.Printf("UpdatedDate different for %s: Old - %s | New - %s\n", item.Project, existingItem.UpdatedDate, item.UpdatedDate)
			}

			if !stringSlicesEqual(existingItem.AssetLinks, assetLinks) {
				fmt.Printf("AssetLinks different for %s\n", item.Project)
			}

			if existingItem.UpdatedDate != item.UpdatedDate || !stringSlicesEqual(existingItem.AssetLinks, assetLinks) {
				linkDiff := stringDifference(assetLinks, existingItem.AssetLinks)
				differences = append(differences, Difference{
					ID:                 item.ID,
					Project:            item.Project,
					ExistingUpdatedDate: existingItem.UpdatedDate,
					NewUpdatedDate:     item.UpdatedDate,
					LinkDiff:           linkDiff,
				})
			}
		}
	}

	return differences
}

func updateBountiesTable(updatedData []Difference, db *mongo.Database) {
	bountiesCollection := db.Collection("bounties")
	for _, data := range updatedData {
		assetLinks := fetchAssetLinksForBounty(data.ID)
		fmt.Printf("Updating bounties table for %s: New UpdatedDate - %s | AssetLinks - %v\n", data.Project, data.NewUpdatedDate, assetLinks)
		filter := bson.M{"project": data.Project}
		update := bson.M{"$set": bson.M{"updatedDate": data.NewUpdatedDate, "assetLinks": assetLinks}}
		_, err := bountiesCollection.UpdateOne(context.TODO(), filter, update)
		if err != nil {
			log.Println("Error updating bounties table:", err)
		}
	}
}

func sendMessageToTelegram(text string) {
	botToken := telegramBotToken
	chatID := telegramChatID
	baseURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload := map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	resp, err := http.PostForm(baseURL, payload)
	if err != nil {
		log.Println("Error sending message to telegram:", err)
		return
	}
	defer resp.Body.Close()

	// You might want to further process the response from Telegram (e.g., check for success)
}

func initializeBountiesTableIfEmpty(bounties []Bounty, db *mongo.Database) {
	bountiesCollection := db.Collection("bounties")
	count, err := bountiesCollection.CountDocuments(context.TODO(), bson.M{})
	if err != nil {
		log.Println("Error counting documents:", err)
		return
	}

	if count == 0 {
		for i := range bounties {
			bounties[i].AssetLinks = fetchAssetLinksForBounty(bounties[i].ID)
		}
		_, err := bountiesCollection.InsertMany(context.TODO(), bounties)
		if err != nil {
			log.Println("Error initializing bounties:", err)
		}
	}
}

func fetchAssetLinksForBounty(bountyID string) []string {
	url := fmt.Sprintf("https://immunefi.com/bounty/%s/", bountyID)
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error fetching bounty:", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("Failed to retrieve the content. HTTP status code: %d\n", resp.StatusCode)
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Println("Error parsing HTML:", err)
		return nil
	}

	var assetLinks []string
	doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
		href, _ := s.Attr("href")
		if containsAny(href, "github.com", "etherscan.io", "testnet.bscscan.com") {
			assetLinks = append(assetLinks, href)
		}
	})

	return assetLinks
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringDifference(a, b []string) []string {
	mb := make(map[string]bool, len(b))
	for _, x := range b {
		mb[x] = true
	}
	var diff []string
	for _, x := range a {
		if !mb[x] {
			diff = append(diff, x)
		}
	}
	return diff
}

func containsAny(str string, substrs ...string) bool {
	for _, s := range substrs {
		if contains(str, s) {
			return true
		}
	}
	return false
}

func contains(str, substr string) bool {
	return bytes.Contains([]byte(str), []byte(substr))
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	user_agent := os.Getenv("USER_AGENT")
	mongo_uri := os.Getenv("MONGO_URI")
	mongo_db := os.Getenv("MONGO_DB")

	func main() {
		err := godotenv.Load()
		if err != nil {
			log.Fatal("Error loading .env file")
		}
	
		user_agent := os.Getenv("USER_AGENT")
		mongo_uri := os.Getenv("MONGO_URI")
		mongo_db := os.Getenv("MONGO_DB")
	
		for {
			url := "https://immunefi.com/explore/"
			sourceCode, err := fetchSourceCode(url, user_agent)
			if err != nil {
				log.Println("Error fetching source code:", err)
				time.Sleep(10 * time.Minute)
				continue
			}
			token := extractTokenFromSource(sourceCode)
			bounties, err := fetchDataUsingToken(token)
			if err != nil {
				log.Println("Error fetching data using token:", err)
				time.Sleep(10 * time.Minute)
				continue
			}
			db, err := connectToDatabase(mongo_uri, mongo_db)
			if err != nil {
				log.Println("Error connecting to database:", err)
				time.Sleep(10 * time.Minute)
				continue
			}
	
			// Initializing bounties table if empty
			err = initializeBountiesTableIfEmpty(bounties, db)
			if err != nil {
				log.Println("Error initializing bounties table:", err)
				time.Sleep(10 * time.Minute)
				continue
			}
	
			// Comparing with existing data
			differences := compareWithExistingData(bounties, db)
			if len(differences) > 0 {
				log.Println("Found differences in the data.")
				insertIntoDiffTable(differences, db)
				updateBountiesTable(differences, db)
			} else {
				log.Println("No differences found.")
			}
	
			time.Sleep(10 * time.Minute)
		}
	}
}