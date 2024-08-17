package main

import (
    "fmt"
    "io/ioutil"
    "log"
    "math/rand"
    "net/http"
    "os"
    "strings"
    "sync"
    "time"

    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func generateImageURL(description string) string {
    randomSeed := ""
    for i := 0; i < 8; i++ {
        randomSeed += string(rand.Intn(10) + 48) // Generate random number seed
    }

    formattedDescription := strings.Join(strings.Fields(description), "%20") // Replace spaces with %20

    return fmt.Sprintf("https://image.pollinations.ai/prompt/%s?nologo=true&seed=%s", formattedDescription, randomSeed)
}

func downloadImage(url string) ([]byte, error) {
    response, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()

    if response.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
    }

    return ioutil.ReadAll(response.Body)
}

func sendImage(bot *tgbotapi.BotAPI, chatID int64, imageData []byte) error {
    tmpFileName := fmt.Sprintf("image_%d.jpg", rand.Intn(1000000))
    tmpFile, err := os.CreateTemp("", tmpFileName)
    if err != nil {
        return err
    }
    defer os.Remove(tmpFile.Name())

    _, err = tmpFile.Write(imageData)
    if err != nil {
        return err
    }

    photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileBytes{Name: tmpFileName, Bytes: imageData})
    _, err = bot.Send(photo)
    return err
}

func processMessage(bot *tgbotapi.BotAPI, update tgbotapi.Update, wg *sync.WaitGroup) {
    defer wg.Done()

    chatID := update.Message.Chat.ID
    description := update.Message.Text

    log.Printf("[%s] %s", update.Message.From.UserName, description)

    msg := tgbotapi.NewMessage(chatID, "Generating your image...")
    sentMsg, _ := bot.Send(msg)

    imageURL := generateImageURL(description)

    imageData, err := downloadImage(imageURL)
    if err != nil {
        log.Printf("Error downloading image: %v", err)
        editMsg := tgbotapi.NewEditMessageText(chatID, sentMsg.MessageID, "Oops! There was an issue generating your image.")
        bot.Send(editMsg)
        return
    }

    err = sendImage(bot, chatID, imageData)
    if err != nil {
        log.Printf("Error sending image: %v", err)
    } else {
        _, err = bot.Request(tgbotapi.DeleteMessageConfig{ChatID: chatID, MessageID: sentMsg.MessageID})
        if err != nil {
            log.Printf("Error deleting message: %v", err)
        }
    }
}

func main() {
    bot, err := tgbotapi.NewBotAPI("YOUR_TELEGRAM_BOT_TOKEN")
    if err != nil {
        log.Panic(err)
    }

    bot.Debug = false

    log.Printf("Authorized on account %s", bot.Self.UserName)

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

        time.Sleep(100 * time.Millisecond)
    }

    wg.Wait()
}
