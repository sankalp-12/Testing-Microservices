package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn      *amqp.Connection
	queueName string
}

type RequestPayload struct {
	Action string      `json:"action"`
	Auth   AuthPayload `json:"auth,omitempty"`
	Log    LogPayload  `json:"log,omitempty"`
}

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LogPayload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

type jsonResponse struct {
	Error   bool   `json:"error"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func NewConsumer(conn *amqp.Connection) (Consumer, error) {
	consumer := Consumer{
		conn: conn,
	}

	err := consumer.setup()
	if err != nil {
		return Consumer{}, err
	}

	return consumer, nil
}

func (consumer *Consumer) setup() error {
	channel, err := consumer.conn.Channel()
	if err != nil {
		return err
	}

	return declareExchange(channel)
}

func (consumer *Consumer) Listen(topics []string) error {
	ch, err := consumer.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	q, err := declareRandomQueue(ch)
	if err != nil {
		return err
	}

	for _, s := range topics {
		ch.QueueBind(
			q.Name,
			s,
			"exchange",
			false,
			nil,
		)

		if err != nil {
			return err
		}
	}

	messages, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		return err
	}

	forever := make(chan bool)
	go func() {
		for d := range messages {
			var payload RequestPayload
			_ = json.Unmarshal(d.Body, &payload)

			go handlePayload(payload)
		}
	}()

	fmt.Printf("Waiting for message [Exchange, Queue] [exchange, %s]\n", q.Name)
	<-forever

	return nil
}

func handlePayload(payload RequestPayload) {
	switch payload.Action {
	case "log":
		// Log whatever we get
		err := logEvent(payload.Log)
		if err != nil {
			log.Println(err)
		}
	case "auth":
		// Authenticate
		err := authEvent(payload.Auth)
		if err != nil {
			log.Println(err)
		}
		// You can have as many cases as you want, as long as you write the logic
	default:
		err := logEvent(payload.Log)
		if err != nil {
			log.Println(err)
		}
	}
}

func logEvent(entry LogPayload) error {
	jsonData, _ := json.MarshalIndent(entry, "", "\t")

	logServiceURL := "http://logger-service/log"

	request, err := http.NewRequest("POST", logServiceURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return err
	}

	return nil
}

func authEvent(entry AuthPayload) error {
	jsonData, _ := json.MarshalIndent(entry, "", "\t")

	authServiceURL := "http://authentication-service/authenticate"

	request, err := http.NewRequest("POST", authServiceURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	// Make sure we get back the correct Status Code
	if response.StatusCode == http.StatusUnauthorized {
		return errors.New("invalid credentials")
	} else if response.StatusCode != http.StatusAccepted {
		return errors.New("error calling auth service")
	}

	// Create a variable that we will read response.Body into
	var jsonFromService jsonResponse

	// Decode the JSON from the Auth-Microservice
	err = json.NewDecoder(response.Body).Decode(&jsonFromService)
	if err != nil {
		return err
	}

	if jsonFromService.Error {
		return err
	}

	return nil
}
