package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/joho/godotenv"
	"github.com/mmcdole/gofeed"
)

// NewsItem represents a single news item
type NewsItem struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Link        string    `json:"link"`
	Source      string    `json:"source"`
	PublishDate time.Time `json:"publish_date"`
	Score       int       `json:"score"` // Relevance score for ranking
}

// WebhookPayload represents the data sent to the webhook
type WebhookPayload struct {
	Timestamp   time.Time  `json:"timestamp"`
	TopNews     []NewsItem `json:"top_news"`
	TrendingNow []string   `json:"trending_topics"`
	Summary     string     `json:"summary"`
}

// Config holds the application configuration
type Config struct {
	WebhookURL       string
	MaxNewsItems     int
	RequestTimeout   time.Duration
	UserAgent        string
	GoogleTrendsURL  string
	TwitterTrendsAPI string
}

// NewsAggregator is the main struct for the news aggregation service
type NewsAggregator struct {
	config Config
	client *http.Client
}

// NewNewsAggregator creates a new instance of NewsAggregator
func NewNewsAggregator(webhookURL string) *NewsAggregator {
	return &NewsAggregator{
		config: Config{
			WebhookURL:      webhookURL,
			MaxNewsItems:    5,
			RequestTimeout:  30 * time.Second,
			UserAgent:       "SpainNewsAggregator/1.0",
			GoogleTrendsURL: "https://trends.google.es/trends/trendingsearches/daily/rss?geo=ES",
		},
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// FetchBBCMundoNews fetches news from BBC Mundo
func (na *NewsAggregator) FetchBBCMundoNews() ([]NewsItem, error) {
	urls := []string{
		"https://feeds.bbci.co.uk/mundo/rss.xml",
		"https://feeds.bbci.co.uk/mundo/noticias/rss.xml",
	}

	var allNews []NewsItem

	for _, url := range urls {
		news, err := na.fetchRSSFeed(url, "BBC Mundo")
		if err != nil {
			log.Printf("Error fetching BBC Mundo feed from %s: %v", url, err)
			continue
		}
		allNews = append(allNews, news...)
	}

	return na.filterSpainNews(allNews), nil
}

// FetchCNNEspanolNews fetches news from CNN en Español
func (na *NewsAggregator) FetchCNNEspanolNews() ([]NewsItem, error) {
	// CNN en Español main page scraping since RSS might not be available
	url := "https://cnnespanol.cnn.com/category/espana/"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", na.config.UserAgent)
	resp, err := na.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var news []NewsItem

	// Parse CNN articles
	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		if i >= 10 { // Limit to 10 articles
			return
		}

		titleElem := s.Find("h3 a").First()
		title := strings.TrimSpace(titleElem.Text())
		link, _ := titleElem.Attr("href")

		if !strings.HasPrefix(link, "http") {
			link = "https://cnnespanol.cnn.com" + link
		}

		description := strings.TrimSpace(s.Find(".news__excerpt").Text())
		if description == "" {
			description = strings.TrimSpace(s.Find("p").First().Text())
		}

		if title != "" && link != "" {
			news = append(news, NewsItem{
				Title:       title,
				Description: description,
				Link:        link,
				Source:      "CNN en Español",
				PublishDate: time.Now(), // CNN doesn't always show dates on listing
			})
		}
	})

	return news, nil
}

// FetchGoogleTrends fetches trending topics from Google Trends Spain
func (na *NewsAggregator) FetchGoogleTrends() ([]string, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(na.config.GoogleTrendsURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing Google Trends feed: %v", err)
	}

	var trends []string
	for i, item := range feed.Items {
		if i >= 10 { // Limit to top 10 trends
			break
		}
		trends = append(trends, item.Title)
	}

	return trends, nil
}

// FetchTwitterTrends would fetch X (Twitter) trends
// Note: This requires Twitter API access which needs authentication
func (na *NewsAggregator) FetchTwitterTrends() ([]string, error) {
	// For demonstration, we'll scrape from a trends aggregator
	url := "https://trends24.in/spain/"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", na.config.UserAgent)
	resp, err := na.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	var trends []string
	doc.Find(".trend-card__title").Each(func(i int, s *goquery.Selection) {
		if i >= 5 { // Top 5 trends
			return
		}
		trend := strings.TrimSpace(s.Text())
		if trend != "" {
			trends = append(trends, trend)
		}
	})

	return trends, nil
}

// fetchRSSFeed is a helper to fetch and parse RSS feeds
func (na *NewsAggregator) fetchRSSFeed(url, source string) ([]NewsItem, error) {
	fp := gofeed.NewParser()
	feed, err := fp.ParseURL(url)
	if err != nil {
		return nil, err
	}

	var news []NewsItem
	for _, item := range feed.Items {
		publishDate := time.Now()
		if item.PublishedParsed != nil {
			publishDate = *item.PublishedParsed
		}

		// Only include news from last 24 hours
		if time.Since(publishDate) > 24*time.Hour {
			continue
		}

		news = append(news, NewsItem{
			Title:       item.Title,
			Description: item.Description,
			Link:        item.Link,
			Source:      source,
			PublishDate: publishDate,
		})
	}

	return news, nil
}

// filterSpainNews filters news items to only include Spain-related content
func (na *NewsAggregator) filterSpainNews(news []NewsItem) []NewsItem {
	spainKeywords := []string{
		"españa", "spain", "español", "española",
		"madrid", "barcelona", "valencia", "sevilla",
		"gobierno español", "pedro sánchez", "rey felipe",
		"la moncloa", "congreso de los diputados",
	}

	var filtered []NewsItem
	for _, item := range news {
		content := strings.ToLower(item.Title + " " + item.Description)

		for _, keyword := range spainKeywords {
			if strings.Contains(content, keyword) {
				item.Score = calculateRelevanceScore(item, spainKeywords)
				filtered = append(filtered, item)
				break
			}
		}
	}

	return filtered
}

// calculateRelevanceScore calculates a relevance score for ranking
func calculateRelevanceScore(item NewsItem, keywords []string) int {
	score := 0
	content := strings.ToLower(item.Title + " " + item.Description)

	// More recent = higher score
	hoursSincePublish := int(time.Since(item.PublishDate).Hours())
	if hoursSincePublish < 1 {
		score += 100
	} else if hoursSincePublish < 6 {
		score += 50
	} else if hoursSincePublish < 12 {
		score += 25
	}

	// Count keyword matches
	for _, keyword := range keywords {
		if strings.Contains(content, keyword) {
			score += 10
		}
	}

	// Title matches are worth more
	titleLower := strings.ToLower(item.Title)
	for _, keyword := range keywords {
		if strings.Contains(titleLower, keyword) {
			score += 20
		}
	}

	return score
}

// rankNewsByRelevance sorts news by relevance score
func (na *NewsAggregator) rankNewsByRelevance(news []NewsItem) []NewsItem {
	// Simple bubble sort for demonstration
	for i := 0; i < len(news)-1; i++ {
		for j := 0; j < len(news)-i-1; j++ {
			if news[j].Score < news[j+1].Score {
				news[j], news[j+1] = news[j+1], news[j]
			}
		}
	}

	// Return top N items
	if len(news) > na.config.MaxNewsItems {
		return news[:na.config.MaxNewsItems]
	}
	return news
}

// AggregateNews combines all news sources and trends
func (na *NewsAggregator) AggregateNews() (*WebhookPayload, error) {
	// Fetch news from different sources
	var allNews []NewsItem

	// BBC Mundo
	bbcNews, err := na.FetchBBCMundoNews()
	if err != nil {
		log.Printf("Error fetching BBC Mundo news: %v", err)
	} else {
		allNews = append(allNews, bbcNews...)
	}

	// CNN en Español
	cnnNews, err := na.FetchCNNEspanolNews()
	if err != nil {
		log.Printf("Error fetching CNN news: %v", err)
	} else {
		allNews = append(allNews, cnnNews...)
	}

	// Rank by relevance
	topNews := na.rankNewsByRelevance(allNews)

	// Fetch trending topics
	var trendingTopics []string

	googleTrends, err := na.FetchGoogleTrends()
	if err != nil {
		log.Printf("Error fetching Google Trends: %v", err)
	} else {
		trendingTopics = append(trendingTopics, googleTrends...)
	}

	twitterTrends, err := na.FetchTwitterTrends()
	if err != nil {
		log.Printf("Error fetching Twitter trends: %v", err)
	} else {
		trendingTopics = append(trendingTopics, twitterTrends...)
	}

	// Remove duplicates from trends
	trendingTopics = removeDuplicates(trendingTopics)

	// Create summary
	summary := fmt.Sprintf("Top %d Spain news items from the last 24 hours. Sources: BBC Mundo, CNN en Español. Trending topics include: %s",
		len(topNews), strings.Join(trendingTopics[:min(5, len(trendingTopics))], ", "))

	return &WebhookPayload{
		Timestamp:   time.Now(),
		TopNews:     topNews,
		TrendingNow: trendingTopics,
		Summary:     summary,
	}, nil
}

// SendToWebhook sends the aggregated news to the specified webhook
func (na *NewsAggregator) SendToWebhook(payload *WebhookPayload) error {
	jsonData, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling payload: %v", err)
	}

	req, err := http.NewRequest("POST", na.config.WebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", na.config.UserAgent)

	resp, err := na.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending webhook: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Printf("Successfully sent news to webhook. Status: %d", resp.StatusCode)
	return nil
}

// Helper functions
func removeDuplicates(items []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Run executes the news aggregation and webhook sending
func (na *NewsAggregator) Run() error {
	log.Println("Starting Spain news aggregation...")

	payload, err := na.AggregateNews()
	if err != nil {
		return fmt.Errorf("error aggregating news: %v", err)
	}

	log.Printf("Aggregated %d news items and %d trending topics",
		len(payload.TopNews), len(payload.TrendingNow))

	// Print summary to console
	fmt.Println("\n=== TOP SPAIN NEWS ===")
	for i, news := range payload.TopNews {
		fmt.Printf("\n%d. %s\n", i+1, news.Title)
		fmt.Printf("   Source: %s\n", news.Source)
		fmt.Printf("   Link: %s\n", news.Link)
		if news.Description != "" {
			fmt.Printf("   %s\n", truncateString(news.Description, 100))
		}
	}

	fmt.Println("\n=== TRENDING TOPICS ===")
	for i, trend := range payload.TrendingNow {
		if i >= 10 {
			break
		}
		fmt.Printf("%d. %s\n", i+1, trend)
	}

	// Send to webhook
	if err := na.SendToWebhook(payload); err != nil {
		return fmt.Errorf("error sending to webhook: %v", err)
	}

	return nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func main() {

	godotenv.Load()

	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		log.Fatalf("WEBHOOK_URL environment variable is not set")
	}

	// Create and run aggregator
	aggregator := NewNewsAggregator(webhookURL)

	if err := aggregator.Run(); err != nil {
		log.Fatal(err)
	}

	log.Println("News aggregation completed successfully!")
}
