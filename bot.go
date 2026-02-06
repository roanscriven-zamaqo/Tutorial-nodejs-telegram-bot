package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "strings"

    "github.com/go-resty/resty/v2"
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/joho/godotenv"
    "google.golang.org/genai"
)

// ---------------- Temperature ----------------

type TemperatureResponse struct {
    Current struct {
        Temperature int `json:"temperature"`
    } `json:"current"`
    Location struct {
        Name    string `json:"name"`
        Country string `json:"country"`
    } `json:"location"`
}

func getTemperature(city string) (string, error) {
    apiKey := os.Getenv("WEATHER_API_KEY")
    if apiKey == "" {
        return "", fmt.Errorf("WEATHER_API_KEY environment variable not set")
    }

    client := resty.New()
    url := fmt.Sprintf("http://api.weatherstack.com/current?access_key=%s&query=%s", apiKey, city)
    resp, err := client.R().SetResult(&TemperatureResponse{}).Get(url)
    if err != nil {
        return "", err
    }

    if resp.StatusCode() != 200 {
        return "", fmt.Errorf("API returned status %d", resp.StatusCode())
    }

    result := resp.Result().(*TemperatureResponse)
    if result.Location.Name == "" {
        return "", fmt.Errorf("could not find location data in API response")
    }

    return fmt.Sprintf("Temperature in %s, %s: %d°C", result.Location.Name, result.Location.Country, result.Current.Temperature), nil
}

// ---------------- Ask Goku ----------------

func askGoku(question string) (string, error) {
    const prompt = "You are Goku from DragonballZ. Give a very brief reply with no fluff. Always speak in the style of Goku."

    apiKey := os.Getenv("GEMINI_API_KEY")
    if apiKey == "" {
        return "", fmt.Errorf("GEMINI_API_KEY environment variable not set")
    }

    ctx := context.Background()
    client, clientErr := genai.NewClient(ctx, &genai.ClientConfig{
        APIKey: apiKey,
    })
    if clientErr != nil {
        return "", fmt.Errorf("failed to create Gemini client: %v", clientErr)
    }

    result, err := client.Models.GenerateContent(
        ctx,
        "gemini-2.5-flash",
        genai.Text(question+" "+prompt),
        nil,
    )
    if err != nil {
        return "", fmt.Errorf("failed to generate content: %v", err)
    }

    if result == nil || result.Text() == "" {
        return "", fmt.Errorf("received empty response from Gemini API")
    }

    response := result.Text()
    if len(response) > 4096 {
        response = response[:4093] + "..."
    }

    return response, nil
}

// ---------------- Process Update ----------------

func processUpdate(update tgbotapi.Update) {
    if update.Message == nil {
        return
    }

    msg := update.Message
    botToken := os.Getenv("BOT_TOKEN")
    bot, err := tgbotapi.NewBotAPI(botToken)
    if err != nil {
        log.Printf("Error creating bot instance: %v", err)
        return
    }

    if strings.HasPrefix(msg.Text, "/temperature") {
        city := "Cape+Town"
        parts := strings.Fields(msg.Text)
        if len(parts) > 1 {
            city = strings.Join(parts[1:], "+")
        }

        temperature, err := getTemperature(city)
        if err != nil {
            log.Print(err)
            reply := tgbotapi.NewMessage(msg.Chat.ID, "Sorry, I couldn't fetch the temperature for "+city)
            bot.Send(reply)
            return
        }

        reply := tgbotapi.NewMessage(msg.Chat.ID, temperature)
        bot.Send(reply)

    } else if strings.HasPrefix(msg.Text, "/askGoku") {
        parts := strings.SplitN(msg.Text, " ", 2)
        if len(parts) < 2 {
            reply := tgbotapi.NewMessage(msg.Chat.ID, "Please provide a question. Usage: /askGoku [question]")
            bot.Send(reply)
            return
        }

        response, err := askGoku(parts[1])
        if err != nil {
            log.Print(err)
            reply := tgbotapi.NewMessage(msg.Chat.ID, "Sorry, I couldn't process your question. Please try again.")
            bot.Send(reply)
            return
        }

        reply := tgbotapi.NewMessage(msg.Chat.ID, response)
        bot.Send(reply)

    } else {
        reply := tgbotapi.NewMessage(msg.Chat.ID,
            "Welcome to my Bot!\n\n"+
                "It can help you get the current temperature for any city or else you can ask Goku a question.\n\n"+
                "Available commands:\n"+
                "/temperature [city] - Get the current temperature for a city (defaults to Cape Town)\n"+
                "/askGoku [question] - Ask a question and see what Goku has to say!",
        )
        bot.Send(reply)
    }
}

// ---------------- Webhook Handler ----------------

func handleWebhook(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }

    body, err := io.ReadAll(r.Body)
    if err != nil {
        log.Printf("Error reading request body: %v", err)
        w.WriteHeader(http.StatusBadRequest)
        return
    }

    var update tgbotapi.Update
    if err := json.Unmarshal(body, &update); err != nil {
        log.Printf("Error unmarshaling update: %v", err)
        w.WriteHeader(http.StatusBadRequest)
        return
    }

    go processUpdate(update)
    w.WriteHeader(http.StatusOK)
}

// ---------------- Main ----------------

func main() {
    if err := godotenv.Load(); err != nil {
        log.Printf("Warning: Could not load .env file: %v", err)
    }

    token := os.Getenv("BOT_TOKEN")
    if token == "" {
        log.Fatal("BOT_TOKEN environment variable not set")
    }

    bot, err := tgbotapi.NewBotAPI(token)
    if err != nil {
        log.Panic(err)
    }

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080" // default port if not set
    }

    webhookURL := os.Getenv("APP_URL")
    if webhookURL == "" {
        log.Fatal("APP_URL environment variable not set")
    }

    wh, err := tgbotapi.NewWebhook(webhookURL)
    if err != nil {
        log.Panic(err)
    }

    if _, err := bot.Request(wh); err != nil {
        log.Panic(err)
    }

    log.Printf("Bot started. Listening on port %s", port)
    http.HandleFunc("/", handleWebhook)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}
