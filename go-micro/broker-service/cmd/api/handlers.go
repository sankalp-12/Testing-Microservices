package main

import (
	"broker/event"
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
)

type RequestPayload struct {
	Action string      `json:"action"`
	Auth   AuthPayload `json:"auth,omitempty"`
	Log    LogPayload  `json:"log,omitempty"`
	Mail   MailPayload `json:"mail,omitempty"`
}

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LogPayload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

type MailPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

func (app *Config) Broker(w http.ResponseWriter, r *http.Request) {
	payload := jsonResponse{
		Error:   false,
		Message: "Hit the broker",
	}

	_ = app.writeJSON(w, http.StatusOK, payload)
}

func (app *Config) HandleSubmission(w http.ResponseWriter, r *http.Request) {
	var requestPayload RequestPayload

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	switch requestPayload.Action {
	case "auth":
		app.authEventViaRabbit(w, requestPayload)
	case "log":
		app.logEventViaRabbit(w, requestPayload)
	case "mail":
		app.sendMail(w, requestPayload.Mail)
	default:
		app.errorJSON(w, errors.New("unknown action"))
	}
}

// func (app *Config) authenticate(w http.ResponseWriter, a AuthPayload) {
// 	// Create some JSON that we will send to the Auth-Microservice
// 	jsonData, _ := json.MarshalIndent(a, "", "\t")

// 	// Call the microservice
// 	request, err := http.NewRequest("POST", "http://authentication-service/authenticate", bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}

// 	client := &http.Client{}
// 	response, err := client.Do(request)
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}
// 	defer response.Body.Close()

// 	// Make sure we get back the correct Status Code
// 	if response.StatusCode == http.StatusUnauthorized {
// 		app.errorJSON(w, errors.New("invalid credentials"))
// 		return
// 	} else if response.StatusCode != http.StatusAccepted {
// 		app.errorJSON(w, errors.New("error calling auth service"))
// 		return
// 	}

// 	// Create a variable that we will read response.Body into
// 	var jsonFromService jsonResponse

// 	// Decode the JSON from the Auth-Microservice
// 	err = json.NewDecoder(response.Body).Decode(&jsonFromService)
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}

// 	if jsonFromService.Error {
// 		app.errorJSON(w, err, http.StatusUnauthorized)
// 		return
// 	}

// 	var payload jsonResponse
// 	payload.Error = false
// 	payload.Message = "Authenticated!"
// 	payload.Data = jsonFromService.Data

// 	app.writeJSON(w, http.StatusAccepted, payload)
// }

// func (app *Config) logItem(w http.ResponseWriter, entry LogPayload) {
// 	// Create some JSON that we will send to the Logger-Microservice
// 	jsonData, _ := json.MarshalIndent(entry, "", "\t")

// 	// Call the microservice
// 	logServiceURL := "http://logger-service/log"

// 	request, err := http.NewRequest("POST", logServiceURL, bytes.NewBuffer(jsonData))
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}

// 	request.Header.Set("Content-Type", "application/json")

// 	client := &http.Client{}
// 	response, err := client.Do(request)
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}
// 	defer response.Body.Close()

// 	if response.StatusCode != http.StatusAccepted {
// 		app.errorJSON(w, errors.New("error calling logger service"))
// 		return
// 	}

// 	var payload jsonResponse
// 	payload.Error = false
// 	payload.Message = "logged"

// 	app.writeJSON(w, http.StatusAccepted, payload)
// }

func (app *Config) sendMail(w http.ResponseWriter, msg MailPayload) {
	// Create some JSON that we will send to the Mail-Microservice
	jsonData, _ := json.MarshalIndent(msg, "", "\t")

	// Call the microservice
	mailServiceURL := "http://mailer-service/send"

	request, err := http.NewRequest("POST", mailServiceURL, bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		app.errorJSON(w, err)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		app.errorJSON(w, errors.New("error calling mail service"))
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "message sent to " + msg.To

	app.writeJSON(w, http.StatusAccepted, payload)
}

// authEventViaRabbit authenticates an event using the authentication-service. It makes the call by pushing the data to RabbitMQ.
func (app *Config) authEventViaRabbit(w http.ResponseWriter, p RequestPayload) {
	a := p.Auth
	err := app.pushToQueue(p, a.Email, a.Password)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "authenticated via RabbitMQ"

	app.writeJSON(w, http.StatusAccepted, payload)
}

// logEventViaRabbit logs an event using the logger-service. It makes the call by pushing the data to RabbitMQ.
func (app *Config) logEventViaRabbit(w http.ResponseWriter, p RequestPayload) {
	l := p.Log
	err := app.pushToQueue(p, l.Name, l.Data)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "logged via RabbitMQ"

	app.writeJSON(w, http.StatusAccepted, payload)
}

// pushToQueue pushes a message into RabbitMQ
func (app *Config) pushToQueue(p RequestPayload, messages ...string) error {
	emitter, err := event.NewEventEmitter(app.Rabbit)
	if err != nil {
		return err
	}

	switch p.Action {
	case "auth":
		payload := AuthPayload{
			Email:    messages[0],
			Password: messages[1],
		}

		j, _ := json.MarshalIndent(&payload, "", "\t")
		err = emitter.Push(string(j), "auth.INFO")
	case "log":
		payload := LogPayload{
			Name: messages[0],
			Data: messages[1],
		}

		j, _ := json.MarshalIndent(&payload, "", "\t")
		err = emitter.Push(string(j), "log.INFO")
	default:
		payload := LogPayload{
			Name: messages[0],
			Data: messages[1],
		}

		j, _ := json.MarshalIndent(&payload, "", "\t")
		err = emitter.Push(string(j), "log.INFO")
	}

	if err != nil {
		return err
	}
	return nil
}

// logItemviaRPC logs an event using the logger-service. It makes the call through RPC.
// func (app *Config) logItemviaRPC(w http.ResponseWriter, l LogPayload) {
// 	client, err := rpc.Dial("tcp", "logger-service:5001")
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}

// 	rpcPayload := RPCPayload{
// 		Name: l.Name,
// 		Data: l.Data,
// 	}

// 	var result string
// 	err = client.Call("RPCServer.LogInfo", rpcPayload, &result)
// 	if err != nil {
// 		app.errorJSON(w, err)
// 		return
// 	}

// 	payload := jsonResponse{
// 		Error:   false,
// 		Message: result,
// 	}

// 	app.writeJSON(w, http.StatusAccepted, payload)

// }
