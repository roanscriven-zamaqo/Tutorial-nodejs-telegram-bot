package main
import (
    "context"
    "fmt"
    "log"
    "os"
    "strings"
    
    "github.com/go-resty/resty/v2"
    tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
    "github.com/joho/godotenv"
    "google.golang.org/genai"
)

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
        genai.Text(question+" "+prompt), // add the system prompt
        nil,
    )
    if err != nil {
        return "", fmt.Errorf("failed to generate content: %v", err)
    }
    
    if result == nil {
        return "", fmt.Errorf("received nil response from Gemini API")
    }
    
    if result.Text() == "" {
        return "", fmt.Errorf("received empty response from Gemini API")
    }
    
    response := result.Text()
    if len(response) > 4096 {
        response = response[:4093] + "..."
    }
    
    return response, nil
}

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

    u := tgbotapi.NewUpdate(0)
    u.Timeout = 60

    // Get the updates channel from the bot
    updates := bot.GetUpdatesChan(u)

    for update := range updates {
        if update.Message == nil {
            continue
        }

        msg := update.Message

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
                continue
            }

            reply := tgbotapi.NewMessage(msg.Chat.ID, temperature)
            bot.Send(reply)

        } else if strings.HasPrefix(msg.Text, "/askGoku") {
            parts := strings.SplitN(msg.Text, " ", 2)
            if len(parts) < 2 {
                reply := tgbotapi.NewMessage(msg.Chat.ID, "Please provide a question. Usage: /askGoku [question]")
                bot.Send(reply)
                continue
            }

            response, err := askGoku(parts[1])
            if err != nil {
                log.Print(err)
                reply := tgbotapi.NewMessage(msg.Chat.ID, "Sorry, I couldn't process your question. Please try again.")
                bot.Send(reply)
                continue
            }

            reply := tgbotapi.NewMessage(msg.Chat.ID, response)
            bot.Send(reply)

        } else {
            reply := tgbotapi.NewMessage(msg.Chat.ID, "Welcome to my Bot!\n\nIt can help you get the current temperature for any city or else you can ask Goku a question.\n\nAvailable commands:\n/temperature [city] - Get the current temperature for a city (defaults to Cape Town)\n/askGoku [question] - Ask a question and see what Goku has to say!")
            bot.Send(reply)
        }
    }
}
