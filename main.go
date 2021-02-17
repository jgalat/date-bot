package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api"
)

const BASE_URL_DAYS = "http://turnos.santafe.gov.ar/turnos/web/frontend.php/turnos/diaslibres/oficina/479/ano/2021/mes"
const BASE_URL_HOURS = "http://turnos.santafe.gov.ar/turnos/web/frontend.php/turnos/ajax/x/1613584768/oficina/479/ano/2021/mes"
const RESERVE_URL = "http://turnos.santafe.gov.ar/turnos/web/frontend.php/turnos/index/pk/7539"

const HISTORY_FILE = "history.json"

type Date struct {
	Month int `json:"month"`
	Day   int `json:"day"`
}

func checkHistory(date Date, history []Date) bool {
	for _, historyDate := range history {
		if historyDate == date {
			return true
		}
	}
	return false
}

func readHistory() ([]Date, error) {
	history := []Date{}
	body, err := ioutil.ReadFile(HISTORY_FILE)
	if err != nil {
		return history, err
	}

	err = json.Unmarshal(body, &history)
	return history, err
}

func writeHistory(history []Date) error {
	data, err := json.Marshal(&history)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(HISTORY_FILE, data, 0)
}

func get(url string) ([]byte, error) {
	response, err := http.Get(url)
	if err != nil {
		return []byte{}, err
	}
	defer response.Body.Close()

	bytes, err := ioutil.ReadAll(response.Body)
	return bytes, err
}

func availableDates(month int) ([]Date, error) {
	url := fmt.Sprintf("%s/%d", BASE_URL_DAYS, month)
	bytes, err := get(url)
	if err != nil {
		return []Date{}, err
	}

	raw := strings.Split(string(bytes), ",")
	if len(raw) < 2 {
		return []Date{}, nil
	}

	dates := make([]Date, len(raw)-2)
	for i, day := range raw[1 : len(raw)-1] {
		d, err := strconv.Atoi(day)
		if err != nil {
			return []Date{}, err
		}
		dates[i] = Date{Month: month, Day: d}
	}

	return dates, nil
}

func availableHours(date Date) ([]string, error) {
	url := fmt.Sprintf("%s/%d/dia/%d", BASE_URL_HOURS, date.Month, date.Day)
	bytes, err := get(url)
	if err != nil {
		return []string{}, err
	}

	r := regexp.MustCompile(`value="[0-9\-:]+"`)
	matches := r.FindAllString(string(bytes), -1)
	clean := make([]string, len(matches))

	for i, match := range matches {
		clean[i] = strings.ReplaceAll(match, `value="`, "")
		clean[i] = strings.ReplaceAll(clean[i], `"`, "")
		clean[i] = fmt.Sprintf(" - %s", clean[i])
	}

	return clean, nil
}

func formatMessage(m map[Date][]string) string {
	msg := "*Hi there! 👋*\nHere are the new available dates:\n"
	for date, times := range m {
		msg = fmt.Sprintf("%s\n*Date %d/%d:*", msg, date.Day, date.Month)
		msg = fmt.Sprintf("%s\n%s\n", msg, strings.Join(times, "\n"))
	}

	msg = fmt.Sprintf("%s\nSave the date [here](%s)!", msg, RESERVE_URL)
	return msg
}

func handleCheck(chatId int64, bot *tgbotapi.BotAPI) error {
	log.Println("Starting check ...")

	nextMonth := int(time.Now().Month()) + 1
	if nextMonth == 13 {
		nextMonth = 1
	}

	history, err := readHistory()
	if err != nil {
		return err
	}

	log.Printf("Checking for month: %d", nextMonth)
	dates, err := availableDates(nextMonth)
	if err != nil {
		return err
	}
	log.Printf("Possible dates for %d: %v", nextMonth, dates)
	possibleTimes := make(map[Date][]string)
	for _, date := range dates {
		if checkHistory(date, history) {
			continue
		}

		history = append(history, date)
		log.Printf("Checking date: %v", date)
		times, err := availableHours(date)
		if err != nil {
			return err
		}
		log.Printf("Available hours: %v", times)
		possibleTimes[date] = times
	}

	if len(possibleTimes) == 0 {
		log.Println("Nothing to notify")
		return nil
	}

	msg := tgbotapi.NewMessage(chatId, formatMessage(possibleTimes))
	msg.ParseMode = "Markdown"

	log.Println("Sending message ...")
	_, err = bot.Send(msg)
	if err != nil {
		return err
	}
	log.Println("Message sent!")

	log.Println("Updating history with new dates")
	writeHistory(history)
	log.Println("History saved!")

	return nil
}

func handleTestBot(bot *tgbotapi.BotAPI) error {
	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		return err
	}

	for update := range updates {
		if update.Message == nil {
			continue
		}
		msg := fmt.Sprintf("[%s] Chat ID: %d", update.Message.From.UserName, update.Message.Chat.ID)
		log.Println(msg)
		chatMessage := tgbotapi.NewMessage(update.Message.Chat.ID, msg)
		bot.Send(chatMessage)
	}

	return nil
}

func main() {
	if len(os.Args) != 2 {
		log.Fatal("go run main.go ['check', 'test-bot']")
	}

	bot, err := tgbotapi.NewBotAPI(os.Getenv("BOT_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}

	switch os.Args[1] {
	case "check":
		chatId, err := strconv.ParseInt(os.Getenv("CHAT_ID"), 10, 64)
		if err != nil {
			log.Fatal(err)
		}

		if err = handleCheck(chatId, bot); err != nil {
			log.Fatal(err)
		}
	case "test-bot":
		if err = handleTestBot(bot); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatal("go run main.go ['check', 'test-bot']")
	}
}
