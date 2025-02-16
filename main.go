package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// generateImageURL creates an image URL for Pollinations AI.
func generateImageURL(description, model string) string {
	randomSeed := fmt.Sprintf("%08d", rand.Intn(100000000)) // Generate an 8-digit random seed
	formattedDescription := url.QueryEscape(description)    // URL-encode description

	if model == "pollinations" {
		return fmt.Sprintf("https://image.pollinations.ai/prompt/%s?nologo=true&seed=%s", formattedDescription, randomSeed)
	} else if model == "freeimage" {
		// For FreeImage.ai, we use a POST request instead.
		return "https://freeimage.ai/api" // Placeholder
	}

	return ""
}

// generateFreeImage makes a POST request to FreeImage.ai API and returns the image data.
func generateFreeImage(description string) ([]byte, error) {
	apiURL := "https://freeimage.ai/images/generate"

	data := url.Values{}
	data.Set("prompt", description)
	data.Set("samples", "1")
	data.Set("size", "512x512")
	data.Set("visibility", "1")

	req, err := http.NewRequest("POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create POST request: %v", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	// Uncomment and modify the following headers if required by the API:
	// req.Header.Set("Cookie", "your-cookie-here")
	// req.Header.Set("X-CSRF-TOKEN", "your-csrf-token-here")
	// req.Header.Set("X-Requested-With", "XMLHttpRequest")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code %d from FreeImage.ai API: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read FreeImage.ai API response: %v", err)
	}

	var jsonResponse map[string]interface{}
	if err := json.Unmarshal(body, &jsonResponse); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v; response body: %s", err, string(body))
	}

	images, found := jsonResponse["images"]
	if !found {
		return nil, fmt.Errorf("JSON response does not contain 'images' field: %v", jsonResponse)
	}

	imagesSlice, ok := images.([]interface{})
	if !ok || len(imagesSlice) == 0 {
		return nil, fmt.Errorf("'images' field is not a valid array or is empty: %v", images)
	}

	imgMap, ok := imagesSlice[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("first element of 'images' is not an object: %v", imagesSlice[0])
	}

	imageURL, exists := imgMap["src"].(string)
	if !exists {
		return nil, fmt.Errorf("'src' field is not present or not a string in: %v", imgMap)
	}

	imageData, err := downloadImage(imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download image from FreeImage.ai: %v", err)
	}

	return imageData, nil
}

// downloadImage downloads the image from the given URL.
func downloadImage(url string) ([]byte, error) {
	response, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to GET image: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code while downloading image: %d", response.StatusCode)
	}

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %v", err)
	}

	return data, nil
}

// sendImage sends the image to a Telegram chat.
func sendImage(bot *tgbotapi.BotAPI, chatID int64, imageData []byte) error {
	tmpFileName := fmt.Sprintf("image_%d.jpg", rand.Intn(1000000))
	tmpFile, err := os.CreateTemp("", tmpFileName)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(imageData)
	if err != nil {
		return fmt.Errorf("failed to write image data to temp file: %v", err)
	}

	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: tmpFileName, Bytes: imageData})
	if _, err := bot.Send(photo); err != nil {
		return fmt.Errorf("failed to send photo via Telegram: %v", err)
	}

	return nil
}

// HelloHandler handles the web server requests.
func HelloHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "index.html")
	} else if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, fmt.Sprintf("Failed to parse form: %v", err), http.StatusBadRequest)
			return
		}

		description := r.FormValue("description")
		model := r.FormValue("model") // "pollinations" or "freeimage"
		var imageData []byte
		var err error

		if model == "pollinations" {
			imageURL := generateImageURL(description, model)
			imageData, err = downloadImage(imageURL)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to generate image using Pollinations: %v", err), http.StatusInternalServerError)
				return
			}
		} else if model == "freeimage" {
			imageData, err = generateFreeImage(description)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to generate image using FreeImage: %v", err), http.StatusInternalServerError)
				return
			}
		} else {
			http.Error(w, "Invalid model selected. Please choose 'pollinations' or 'freeimage'.", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(imageData)
	} else {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
	}
}

// processMessage handles Telegram messages.
func processMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update, wg *sync.WaitGroup) {
	defer wg.Done()

	chatID := update.Message.Chat.ID
	// Expecting format: /generate <model> <prompt>
	parts := strings.Fields(update.Message.Text)
	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(chatID, "Usage: /generate <model> <prompt>\nModels: pollinations, freeimage")
		bot.Send(msg)
		return
	}

	// Remove the command part if it exists (e.g., "/generate")
	if strings.HasPrefix(parts[0], "/generate") {
		parts = parts[1:]
	}

	if len(parts) < 2 {
		msg := tgbotapi.NewMessage(chatID, "Usage: /generate <model> <prompt>\nModels: pollinations, freeimage")
		bot.Send(msg)
		return
	}

	model := strings.ToLower(parts[0])
	description := strings.Join(parts[1:], " ")

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Generating your image using %s...", model))
	sentMsg, _ := bot.Send(msg)

	var imageData []byte
	var err error

	if model == "pollinations" {
		imageURL := generateImageURL(description, model)
		imageData, err = downloadImage(imageURL)
		if err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, fmt.Sprintf("Failed to generate image using Pollinations: %v", err))
			bot.Send(editMsg)
			return
		}
	} else if model == "freeimage" {
		imageData, err = generateFreeImage(description)
		if err != nil {
			editMsg := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, fmt.Sprintf("Failed to generate image using FreeImage: %v", err))
			bot.Send(editMsg)
			return
		}
	} else {
		editMsg := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "Invalid model. Please choose 'pollinations' or 'freeimage'.")
		bot.Send(editMsg)
		return
	}

	err = sendImage(bot, chatID, imageData)
	if err != nil {
		log.Printf("Error sending image: %v", err)
	} else {
		if _, err = bot.Request(tgbotapi.DeleteMessageConfig{ChatID: chatID, MessageID: sentMsg.MessageID}); err != nil {
			log.Printf("Error deleting message: %v", err)
		}
	}
}

func main() {
	// Get port from environment or default to 8080.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start the web server in a separate goroutine.
	go func() {
		http.HandleFunc("/", HelloHandler)
		log.Println("Web server listening on port", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Fatalf("Failed to start web server: %v", err)
		}
	}()

	// Initialize Telegram bot.
	botToken := "your token here"
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Panicf("Failed to create Telegram bot: %v", err)
	}

	bot.Debug = false
	log.Printf("Authorized on Telegram account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := bot.GetUpdatesChan(u)

	var wg sync.WaitGroup
	for update := range updates {
		if update.Message == nil {
			continue
		}

		wg.Add(1)
		go processMessage(bot, update, &wg)
		time.Sleep(100 * time.Millisecond) // Simple rate limiting.
	}

	wg.Wait()
}
