package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"sync"
)

const URL = "https://go-challenge.skip.money"
const COLLECTION = "azuki"
const COLOR_GREEN = "\033[32m"
const COLOR_RED = "\033[31m"
const COLOR_RESET = "\033[0m"
const NUM_WORKER = 5

var logger *log.Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

type Token struct {
	id    int
	attrs map[string]string
}

type RarityScorecard struct {
	rarity float64
	id     int
}

type Collection struct {
	count int
	url   string
}

type CollectionStats struct {
	*Collection
	ValuesInCate map[string]map[string]interface{}
	CountTokens  map[string]uint64
}

func getToken(tid int, colUrl string) *Token {
	url := fmt.Sprintf("%s/%s/%d.json", URL, colUrl, tid)
	res, err := http.Get(url)
	if err != nil {
		logger.Println(string(COLOR_RED), fmt.Sprintf("Error getting token %d :", tid), err, string(COLOR_RESET))
		return &Token{}
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		logger.Println(string(COLOR_RED), fmt.Sprintf("Error reading response for token %d :", tid), err, string(COLOR_RESET))
		return &Token{}
	}
	attrs := make(map[string]string)
	json.Unmarshal(body, &attrs)
	return &Token{
		id:    tid,
		attrs: attrs,
	}
}

func worker(workerId int, wg *sync.WaitGroup, colUrl string, jobs <-chan int, results chan<- *Token) {
	defer wg.Done()
	for id := range jobs {
		token := getToken(id, colUrl)
		if token != nil {
			logger.Println(string(COLOR_GREEN), fmt.Sprintf("[worker-%d] Getting token %d", workerId, id), string(COLOR_RESET))
			results <- token
		}
	}
}

func getTokens(col Collection) ([]*Token, *CollectionStats) {
	jobs := make(chan int)
	results := make(chan *Token)

	// consume downloaded token info
	var (
		wg1             sync.WaitGroup
		tokens          = make([]*Token, col.count)
		collectionStats = &CollectionStats{
			Collection:   &col,
			ValuesInCate: make(map[string]map[string]interface{}, 0), // ValuesInCate should be with key is cate (head, background,...) and value is attribute such as (green barret, red mustache)
			CountTokens:  make(map[string]uint64, 0),                 // count number of each token in all collections
		}
	)
	wg1.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			select {
			case token, ok := <-results:
				if !ok {
					return
				}
				if token == nil {
					return
				}
				tokens[token.id] = token
				for cate, attr := range token.attrs {
					// put all different values of each cate into map
					val, ok := collectionStats.ValuesInCate[cate]
					if !ok {
						val = make(map[string]interface{}, 0)
					}
					val[attr] = nil
					collectionStats.ValuesInCate[cate] = val

					// count number of each token
					count, ok := collectionStats.CountTokens[attr]
					if !ok {
						count = 1
					} else {
						count++
					}
					collectionStats.CountTokens[attr] = count
				}
			}
		}
	}(&wg1)

	// concurrency download token info
	var wg2 sync.WaitGroup
	for i := 0; i < NUM_WORKER; i++ {
		wg2.Add(1)
		go worker(i, &wg2, col.url, jobs, results)
	}

	// send jobs (collection id) to channel for worker
	for i := 0; i < col.count; i++ {
		jobs <- i
	}
	// close job channel
	close(jobs)
	wg2.Wait()

	// close result channel
	close(results)
	wg1.Wait()

	return tokens, collectionStats
}

func computeRarityScore(collectionStats *CollectionStats, token *Token) *RarityScorecard {
	var rarity float64 = 0
	for cate, attr := range token.attrs {
		countWithValue, ok1 := collectionStats.CountTokens[attr]
		valuesInCategory, ok2 := collectionStats.ValuesInCate[cate]
		if ok1 && ok2 {
			rarity += 1 / (float64(countWithValue) * float64(len(valuesInCategory)))
		}
	}
	return &RarityScorecard{
		id:     token.id,
		rarity: rarity,
	}
}

func main() {
	azuki := Collection{
		count: 100,      // 10000,
		url:   "azuki1", // COLLECTION,
	}

	// download data
	tokens, collectionStats := getTokens(azuki)

	// compute rarity
	var rariryScorecards = make([]*RarityScorecard, 0, len(tokens))
	for _, token := range tokens {
		//fmt.Println(token)
		scorecard := computeRarityScore(collectionStats, token)
		rariryScorecards = append(rariryScorecards, scorecard)
	}

	// return top 5 most rare tokens
	sort.SliceStable(rariryScorecards, func(i, j int) bool {
		return rariryScorecards[i].rarity > rariryScorecards[j].rarity
	})
	fmt.Println("Top 5 most rare tokens:")
	for i := 0; i < 5; i++ {
		fmt.Println(fmt.Sprintf("Top %d: Token id %d, Rarity %v", i+1, rariryScorecards[i].id, rariryScorecards[i].rarity))
	}
}
