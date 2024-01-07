# Anti Jailbreak Site Chatbot with GPT-4 Turbo

This project is an attempt at preventing a "$1 chevy sale incident", by using two instances of ChatGPT in order to answer questions. It is very early stage and is overzealous at blocking innocent questions and inputs.

## How to use

```shell
go run .
```
will launch a webserver on port 7129 or PORT if the environment variable is set.
You need to provide your OPENAI_API_KEY as an environment variable or put it into a .env file.
The chat interface is not particularly great and shows no indication that a reply is in the works, just wait a few seconds for it to appear.

## How it works
It first sends the users input to a GPT-4 Turbo instance, and this instance checks for any issues and rewrites the prompt safely for the second instance. The second instance then answers this question if it was considered "safe". There are also tripwires in the prompt to detect an attempted prompt leak.