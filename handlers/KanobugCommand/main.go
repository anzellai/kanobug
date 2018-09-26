package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
)

const (
	handler     = "KanobugCommand"
	apiEndpoint = "https://slack.com/api/dialog.open"
)

// Response is of type APIGatewayProxyResponse since we're leveraging the
// AWS Lambda Proxy Request functionality (default behavior)
//
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

// ProxyRequest struct ...
type ProxyRequest events.APIGatewayProxyRequest

// Request is the proxy request from lambda
type Request struct {
	Token       string `json:"token"`
	TeamID      string `json:"team_id"`
	TeamDomain  string `json:"team_domain"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	UserID      string `json:"user_id"`
	UserName    string `json:"user_name"`
	Text        string `json:"text"`
	TriggerID   string `json:"trigger_id"`
	ResponseURL string `json:"response_url"`
}

// Payload struct type ...
type Payload struct {
	TriggerID string `json:"trigger_id"`
	Dialog    Dialog `json:"dialog"`
}

// Dialog struct type ...
type Dialog struct {
	Title       string    `json:"title"`
	CallbackID  string    `json:"callback_id"`
	SubmitLabel string    `json:"submit_label"`
	Elements    []Element `json:"elements"`
}

// Element struct type ...
type Element struct {
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Value    string   `json:"value"`
	Hint     string   `json:"hint"`
	Options  []Option `json:"options"`
	Optional bool     `json:"optional"`
}

// Option struct type ...
type Option struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, r ProxyRequest) (Response, error) {
	log.Printf("%s.Handler - invoke: %+v", handler, r)
	form, err := url.Parse("?" + r.Body)
	if err != nil {
		log.Printf("%s.Handler - unmarhsal error: %+v", handler, err)
	}
	query, _ := url.ParseQuery(form.RawQuery)
	request := Request{
		Token:       query["token"][0],
		TeamID:      query["team_id"][0],
		TeamDomain:  query["team_domain"][0],
		ChannelID:   query["channel_id"][0],
		ChannelName: query["channel_name"][0],
		UserID:      query["user_id"][0],
		UserName:    query["user_name"][0],
		Text:        query["text"][0],
		TriggerID:   query["trigger_id"][0],
		ResponseURL: query["response_url"][0],
	}
	log.Printf("%s.Handler - invoke: %+v, for: %s, trigger_id: %s", handler, request, request.Text, request.TriggerID)
	if request.Token != os.Getenv("SLACK_VERIFICATION_TOKEN") {
		err = errors.New("invalid verification token")
		return Response{
			StatusCode:      400,
			IsBase64Encoded: false,
			Body:            fmt.Sprintf("%s submitting - error: %v", handler, err),
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}, err
	}
	payload, err := json.Marshal(Payload{
		TriggerID: request.TriggerID,
		Dialog: Dialog{
			Title:       "Report a Bug",
			CallbackID:  "report-bug",
			SubmitLabel: "Submit",
			Elements: []Element{
				Element{
					Label: "Summarise the Problem",
					Type:  "text",
					Name:  "summary",
					Value: request.Text,
					Hint:  "A sentence to summarise the problem",
				},
				Element{
					Label: "Product",
					Type:  "select",
					Name:  "product",
					Options: []Option{
						Option{
							Label: "Harry Potter Coding Kit",
							Value: "harry_potter_coding_kit",
						},
						Option{
							Label: "Computer Kit Touch",
							Value: "computer_kit_touch",
						},
						Option{
							Label: "Computer Kit 2018",
							Value: "computer_kit_2018",
						},
						Option{
							Label: "Pixel Kit",
							Value: "pixel_kit",
						},
						Option{
							Label: "Motion Sensor Kit",
							Value: "motion_sensor_kit",
						},
					},
				},
				Element{
					Label:    "Any more details?",
					Type:     "textarea",
					Name:     "details",
					Hint:     "If you can help us reproduce the bug, that'd be grand.",
					Optional: true,
				},
			},
		},
	})
	if err != nil {
		log.Printf("%s.Handler - error marshalling dialog request: %v", handler, err)
	} else {
		req, reqErr := http.NewRequest("POST", apiEndpoint, bytes.NewBuffer(payload))
		if reqErr != nil {
			log.Printf("%s.Handler - error sending dialog request: %v", handler, reqErr)
			err = reqErr
		} else {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_ACCESS_TOKEN"))
			client := &http.Client{}
			response, respErr := client.Do(req)
			if respErr != nil {
				log.Printf("%s.Handler - error receiving dialog response: %v", handler, reqErr)
				err = respErr
			} else {
				defer response.Body.Close()
				var status struct {
					OK    bool   `json:"ok"`
					Error string `json:"error"`
				}
				err = json.NewDecoder(response.Body).Decode(&status)
				log.Printf("%s.Handler - ok: %t, error: %s, err: %v", handler, status.OK, status.Error, err)
			}
		}
	}

	resp := Response{
		StatusCode:      200,
		IsBase64Encoded: false,
		Body:            "",
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
	}

	return resp, nil
}

func main() {
	lambda.Start(Handler)
}
