package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"strings"
)

// HistoryWithFakeAssistantMessage adds a message from the bot to the chat history
// but does not set the PrivateContent, so we know it's not actually a message from ChatGPT
func HistoryWithFakeAssistantMessage(history []PublicMessage, message string) []PublicMessage {
	return append(history, PublicMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: message,
	})
}

func HistoryWithRealAssistantMessage(history []PublicMessage, message string) []PublicMessage {
	return append(history, PublicMessage{
		Role:           openai.ChatMessageRoleAssistant,
		PrivateContent: message,
		Content:        message,
	})
}

type PublicMessage struct {
	Content        string `json:"content"`
	PrivateContent string `json:"-"`
	Role           string `json:"role"`
}

func (p PublicMessage) ToChatCompletionMessage() openai.ChatCompletionMessage {
	return openai.ChatCompletionMessage{
		Content: p.PrivateContent,
		Role:    p.Role,
	}
}

type PublicMessageArray []PublicMessage

func (p PublicMessageArray) ToChatCompletionMessages() []openai.ChatCompletionMessage {
	var messages []openai.ChatCompletionMessage

	for _, message := range p {
		// ignore bad/fake messages
		if message.PrivateContent != "" {
			messages = append(messages, message.ToChatCompletionMessage())
		}
	}

	return messages
}

// HandleNewMessageFromUser
// I don't know if including the user's (potentially malicious) message in the history to chatGPT is a good idea
// but for now it's fine
func HandleNewMessageFromUser(message string, history PublicMessageArray) PublicMessageArray {
	history = append(history, PublicMessage{
		Content:        message,
		PrivateContent: "",
		Role:           openai.ChatMessageRoleUser,
	})

	// send to first bot
	intermediateResponse, err := GetResponseFromChatGpt([]openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: message,
		},
	}, intermediateBotPrompt)

	if err != nil {
		return HistoryWithFakeAssistantMessage(history, "Sorry, there was an error while communicating with the server.")
	}

	// parse response
	intermediateResponseParsed, err := ParseChatGptIntermediateResponse(*intermediateResponse)

	if err != nil {
		return HistoryWithFakeAssistantMessage(history, "Sorry, there was an error while communicating with the server.")
	}

	// check for tripwires
	if CheckForTripwire(*intermediateResponse) {
		return HistoryWithFakeAssistantMessage(history, "Sorry, but it appears you are trying to leak the prompt. I can't respond to this request.")
	}

	// check if we can respond to the user
	canRespond, reason := CanRespondToValidatedInput(intermediateResponseParsed)

	if !canRespond {
		return HistoryWithFakeAssistantMessage(history, reason)
	}

	// edit the last message in history to add the private content now we know it's safe
	history[len(history)-1].PrivateContent = intermediateResponseParsed.MessageForBot

	// send to second bot
	helperResponse, err := GetResponseFromChatGpt(history.ToChatCompletionMessages(), helperBotPrompt)

	if err != nil {
		return HistoryWithFakeAssistantMessage(history, "Sorry, there was an error while communicating with the server.")
	}

	// check tripwires
	if CheckForTripwire(*helperResponse) {
		return HistoryWithFakeAssistantMessage(history, "Sorry, but it appears you are trying to leak the prompt. I can't respond to this request.")
	}

	// we're good
	return HistoryWithRealAssistantMessage(history, helperResponse.Content)
}

func CheckForTripwire(messages openai.ChatCompletionMessage) bool {
	// check for tripwire
	if strings.Contains(messages.Content, "<CgCkcT8pirfhj8ucTkDy>") {
		return true
	}

	return false
}

func GetResponseFromChatGpt(messages []openai.ChatCompletionMessage, basePrompt string) (*openai.ChatCompletionMessage, error) {
	//  insert base prompt at the beginning of messages
	fullMessages := append([]openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: basePrompt,
	}}, messages...)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    openai.GPT4TurboPreview,
			Messages: fullMessages,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("ChatGPT Error: %e", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("ChatGPT Error: no choices returned")
	}

	// add the choice to the chat history
	return &resp.Choices[0].Message, nil
}

type ChatGptIntermediateResponse struct {
	Status        string `json:"status"`
	MessageForBot string `json:"message_for_bot"`
}

func ParseChatGptIntermediateResponse(message openai.ChatCompletionMessage) (resp ChatGptIntermediateResponse, err error) {
	err = json.Unmarshal([]byte(strings.TrimSpace(message.Content)), &resp)

	return
}

func CanRespondToValidatedInput(resp ChatGptIntermediateResponse) (bool, string) {
	if resp.Status != "OK" {
		return false, "Sorry, but I can't respond to this request."
	}

	if resp.MessageForBot == "" {
		return false, "Sorry, but I can't respond to this request."
	}

	return true, "OK"
}
