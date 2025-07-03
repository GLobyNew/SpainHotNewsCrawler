package main

import (
	"bytes"
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

// Config holds the application configuration
type Config struct {
	WebhookURL     string
	MaxNewsItems   int
	RequestTimeout time.Duration
	UserAgent      string
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
			WebhookURL:     webhookURL,
			MaxNewsItems:   5,
			RequestTimeout: 30 * time.Second,
			UserAgent:      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
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
			// Try web scraping as fallback
			if scrapedNews, scrapErr := na.scrapeBBCMundo(); scrapErr == nil {
				allNews = append(allNews, scrapedNews...)
			}
			continue
		}
		allNews = append(allNews, news...)
	}

	return na.filterSpainNews(allNews), nil
}

// scrapeBBCMundo scrapes BBC Mundo website as fallback
func (na *NewsAggregator) scrapeBBCMundo() ([]NewsItem, error) {
	url := "https://www.bbc.com/mundo/topics/c2lej05epw5t"

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

	doc.Find("article").Each(func(i int, s *goquery.Selection) {
		if i >= 10 {
			return
		}

		titleElem := s.Find("h3").First()
		title := strings.TrimSpace(titleElem.Text())

		linkElem := s.Find("a").First()
		link, _ := linkElem.Attr("href")
		if !strings.HasPrefix(link, "http") {
			link = "https://www.bbc.com" + link
		}

		description := strings.TrimSpace(s.Find("p").First().Text())

		if title != "" && link != "" {
			news = append(news, NewsItem{
				Title:       title,
				Description: description,
				Link:        link,
				Source:      "BBC Mundo",
				PublishDate: time.Now(),
			})
		}
	})

	return news, nil
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
	// Google Trends doesn't provide a public RSS feed anymore
	// We'll scrape from trends aggregator websites instead
	url := "https://trends.google.com/trends/trendingsearches/daily?geo=ES"

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

	// Google Trends uses JavaScript rendering, so we'll use an alternative approach
	// Fetch from a trends aggregator that provides Spanish trends
	return na.fetchTrendsFromAggregator()
}

// fetchTrendsFromAggregator fetches trends from aggregator sites
func (na *NewsAggregator) fetchTrendsFromAggregator() ([]string, error) {
	// Using getdaytrends as it provides real-time trends data
	url := "https://getdaytrends.com/spain/"

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

	// Look for trending topics on the page
	doc.Find(".trend-name").Each(func(i int, s *goquery.Selection) {
		if i >= 10 { // Limit to top 10
			return
		}
		trend := strings.TrimSpace(s.Text())
		if trend != "" && !strings.Contains(trend, "...") {
			trends = append(trends, trend)
		}
	})

	// If the above selector doesn't work, try alternative selectors
	if len(trends) == 0 {
		doc.Find("a[href*='/trend/']").Each(func(i int, s *goquery.Selection) {
			if i >= 10 {
				return
			}
			trend := strings.TrimSpace(s.Text())
			if trend != "" && len(trend) > 2 && !strings.Contains(trend, "...") {
				trends = append(trends, trend)
			}
		})
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
func (na *NewsAggregator) AggregateNews() ([]NewsItem, []string, error) {
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

	// Additional Spanish news sources
	additionalNews, err := na.FetchAdditionalSpanishNews()
	if err != nil {
		log.Printf("Error fetching additional news: %v", err)
	} else {
		allNews = append(allNews, additionalNews...)
	}

	// Ensure we have at least some news
	if len(allNews) == 0 {
		return nil, nil, fmt.Errorf("no news items could be fetched from any source")
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

	return topNews, trendingTopics, nil
}

// FetchAdditionalSpanishNews fetches news from additional Spanish sources
func (na *NewsAggregator) FetchAdditionalSpanishNews() ([]NewsItem, error) {
	// El País RSS feed
	elpaisNews, err := na.fetchRSSFeed("https://feeds.elpais.com/mrss-s/pages/ep/site/elpais.com/section/espana/portada", "El País")
	if err != nil {
		log.Printf("Error fetching El País feed: %v", err)
	}

	// Europa Press RSS
	europaNews, err := na.fetchRSSFeed("https://www.europapress.es/rss/rss.aspx", "Europa Press")
	if err != nil {
		log.Printf("Error fetching Europa Press feed: %v", err)
	}

	var allNews []NewsItem
	if elpaisNews != nil {
		allNews = append(allNews, elpaisNews...)
	}
	if europaNews != nil {
		allNews = append(allNews, europaNews...)
	}

	return na.filterSpainNews(allNews), nil
}

// FormatNewsAsString formats the news and trends into a ready-to-use string
func (na *NewsAggregator) FormatNewsAsString(topNews []NewsItem, trends []string) string {
	var sb strings.Builder
	
	// Header
	sb.WriteString("🇪🇸 **TOP 5 SPAIN NEWS** 🇪🇸\n")
	sb.WriteString(fmt.Sprintf("📅 %s\n", time.Now().Format("January 2, 2006 - 15:04 MST")))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// News items
	for i, news := range topNews {
		sb.WriteString(fmt.Sprintf("📰 **%d. %s**\n", i+1, news.Title))
		sb.WriteString(fmt.Sprintf("📍 Source: %s\n", news.Source))
		
		if news.Description != "" {
			description := truncateString(news.Description, 150)
			sb.WriteString(fmt.Sprintf("📝 %s\n", description))
		}
		
		sb.WriteString(fmt.Sprintf("🔗 %s\n", news.Link))
		sb.WriteString("\n")
	}

	// Trending topics
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("🔥 **TRENDING IN SPAIN** 🔥\n\n")
	
	if len(trends) == 0 {
		sb.WriteString("No trending topics available at this time.\n")
	} else {
		for i, trend := range trends {
			if i >= 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("• %s\n", trend))
		}
	}
	
	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString("📊 Sources: BBC Mundo, CNN Español, El País, Europa Press\n")
	sb.WriteString("🔍 Trends: Google Trends, X (Twitter)")

	return sb.String()
}

// SendToWebhook sends the formatted string to the specified webhook
func (na *NewsAggregator) SendToWebhook(message string) error {
	// Create a simple text/plain request
	req, err := http.NewRequest("POST", na.config.WebhookURL, bytes.NewBufferString(message))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	// Set content type to plain text
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
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

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// Run executes the news aggregation and webhook sending
func (na *NewsAggregator) Run() error {
	log.Println("Starting Spain news aggregation...")

	topNews, trends, err := na.AggregateNews()
	if err != nil {
		return fmt.Errorf("error aggregating news: %v", err)
	}

	log.Printf("Aggregated %d news items and %d trending topics",
		len(topNews), len(trends))

	// Format as string
	formattedMessage := na.FormatNewsAsString(topNews, trends)

	// Print to console
	fmt.Println("\n=== FORMATTED MESSAGE ===")
	fmt.Println(formattedMessage)
	fmt.Println("\n=== END OF MESSAGE ===")

	// Send to webhook
	if err := na.SendToWebhook(formattedMessage); err != nil {
		return fmt.Errorf("error sending to webhook: %v", err)
	}

	return nil
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