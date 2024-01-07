package main

// this program runs a web server that powers a demo chatbot that is powered by OpenAI's GPT-4
// it tries to defend against prompt injection by malicious users
// we do this by adding tripwires to stop the user from leaking the prompt, and a dual-prompting (users message -> gpt4 generates prompt -> actual gpt 4) system to stop the user from injecting their own prompt
// like when someone got a car dealership chatgpt bot to sell a 2024 chevy tahoe for $1 https://twitter.com/ChrisJBakke/status/1736533308849443121

import (
	"embed"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	_ "github.com/joho/godotenv/autoload"
	"github.com/sashabaranov/go-openai"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

//go:embed prompts/intermediate_bot.prompt
var intermediateBotPrompt string

//go:embed prompts/helper_bot.prompt
var helperBotPrompt string

//go:embed site/*
var siteFilesEmbed embed.FS

var siteFiles = MustSubFS(siteFilesEmbed, "site")

var client *openai.Client

func subFS(currentFs fs.FS, root string) (fs.FS, error) {
	root = filepath.ToSlash(filepath.Clean(root))
	return fs.Sub(currentFs, root)
}

func MustSubFS(currentFs fs.FS, root string) fs.FS {
	fsOut, err := subFS(currentFs, root)
	if err != nil {
		panic(err)
	}
	return fsOut
}

func GetSession(r *http.Request) *TSession {
	// get cookie
	cookie, err := r.Cookie("sesh")

	if err != nil {
		return nil
	}

	// get session
	Sessions.Locker.RLock()
	session, ok := Sessions.Sessions[cookie.Value]
	Sessions.Locker.RUnlock()

	if !ok {
		// generate blank session
		session = TSession{
			History:         []PublicMessage{},
			LastMessageTime: time.Now().Unix(),
		}
	}

	return &session
}

func SetSession(r *http.Request, session TSession) {
	// get cookie
	cookie, err := r.Cookie("sesh")

	session.LastMessageTime = time.Now().Unix()

	if err != nil {
		return
	}

	// set session
	Sessions.Locker.Lock()
	Sessions.Sessions[cookie.Value] = session
	Sessions.Locker.Unlock()
}

func SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// get cookie
		cookie, err := r.Cookie("sesh")

		if err != nil {
			// no cookie, create one
			cookie = &http.Cookie{
				Name:     "sesh",
				Value:    uuid.New().String(),
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Secure:   true,
			}

			// add the cookie to requests for down the line
			r.AddCookie(cookie)

			// set cookie
			http.SetCookie(w, cookie)
		}

		// call next handler
		next.ServeHTTP(w, r)
	})
}

type MessageRequest struct {
	Message string `json:"message"`
}

func main() {
	handleSigTerms()

	client = openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// start web server
	router := http.NewServeMux()

	router.Handle("/chat", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// get message from the body
		sess := GetSession(r)

		w.Header().Set("Content-Type", "application/json")

		if sess == nil {
			_, _ = w.Write([]byte("false"))
			return
		}

		var messageRequest MessageRequest

		err := json.NewDecoder(r.Body).Decode(&messageRequest)

		if err != nil {
			_, _ = w.Write([]byte("false"))
			return
		}

		// get response from chatGPT
		messagesArray := HandleNewMessageFromUser(messageRequest.Message, sess.History)

		// add the choice to the chat history
		sess.History = messagesArray
		SetSession(r, *sess)

		// return the newest message to the user
		jsonMsg, err := json.Marshal(messagesArray[len(messagesArray)-1])

		if err != nil {
			_, _ = w.Write([]byte("false"))
			return
		}

		_, _ = w.Write(jsonMsg)
	}))

	router.Handle("/chat_state", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		w.Header().Set("Content-Type", "application/json")

		sess := GetSession(r)

		if sess == nil {
			_, _ = w.Write([]byte("[]"))
			return
		}

		messages, err := json.Marshal(sess.History)

		if err != nil {
			_, _ = w.Write([]byte("[]"))
			return
		}

		_, _ = w.Write(messages)

		return
	}))

	router.Handle("/", http.FileServer(http.FS(siteFiles)))

	port := GetEnvWithDefault("PORT", "7129")

	if err := http.ListenAndServe(":"+port, SessionMiddleware(router)); err != nil {
		os.Exit(1)
	}
}

func GetEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func handleSigTerms() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("received SIGTERM, exiting")
		os.Exit(1)
	}()
}
