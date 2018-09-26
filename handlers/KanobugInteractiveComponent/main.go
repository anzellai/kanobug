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
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

const (
	handler     = "KanobugInteractiveComponent"
	apiEndpoint = "https://slack.com/api/dialog.open"
	apiWebhook  = "https://hooks.slack.com/services/%s"
	jiraHost    = "https://%s/rest/api/2/issue/"
)

// Response is of type APIGatewayProxyResponse since we're leveraging the
// AWS Lambda Proxy Request functionality (default behavior)
//
// https://serverless.com/framework/docs/providers/aws/events/apigateway/#lambda-proxy-integration
type Response events.APIGatewayProxyResponse

// ProxyRequest event type ...
type ProxyRequest events.APIGatewayProxyRequest

// Request is the proxy request from lambda
type Request struct {
	Type        string     `json:"type"`
	Submission  submission `json:"submission"`
	CallbackID  string     `json:"callback_id"`
	User        user       `json:"user"`
	ActionTS    string     `json:"action_ts"`
	Token       string     `json:"token"`
	ResponseURL string     `json:"response_url"`
}

type submission struct {
	Summary string `json:"summary"`
	Product string `json:"product"`
	Details string `json:"details"`
}

type user struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Bug is the BUG struct type ...
type Bug struct {
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Summary   string    `json:"summary"`
	Product   string    `json:"product"`
	Details   string    `json:"details"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	TTL       int64     `json:"ttl"`
}

// ProductName return title case product
func (bug Bug) ProductName() string {
	return strings.ToTitle(strings.Replace(bug.Product, "_", " ", -1))
}

// GetDB return DDB handle
func GetDB() (srv *dynamodb.DynamoDB, err error) {
	region := os.Getenv("REGION")
	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	if err != nil {
		return
	}
	srv = dynamodb.New(sess)
	return
}

// ToBug transform request details to Bug
func (request Request) ToBug() Bug {
	details := request.Submission.Details
	if len(details) == 0 {
		details = "N/A"
	}
	now := time.Now()
	bug := Bug{
		UserID:    request.User.ID,
		UserName:  request.User.Name,
		Summary:   request.Submission.Summary,
		Product:   request.Submission.Product,
		Details:   details,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return bug
}

// PutItem upsert BUG instance to db
func (request Request) PutItem() (err error) {
	bug := request.ToBug()
	defer log.Printf(
		"%s.PutItem (%s/%s/%s/%s) - error: %v",
		handler,
		bug.UserID,
		bug.UserName,
		bug.Summary,
		bug.Product,
		err,
	)
	srv, err := GetDB()
	if err != nil {
		return
	}
	bug.TTL = bug.UpdatedAt.AddDate(0, 0, 7).Unix()
	item, err := dynamodbattribute.MarshalMap(bug)
	if err != nil {
		return
	}
	input := &dynamodb.PutItemInput{
		Item:      item,
		TableName: aws.String(os.Getenv("TABLE_NAME")),
	}
	_, err = srv.PutItem(input)
	return
}

// Handler is our lambda handler invoked by the `lambda.Start` function call
func Handler(ctx context.Context, r ProxyRequest) (Response, error) {
	log.Printf("%s.Handler - submitted: %+v", handler, r)
	form, err := url.Parse("?" + r.Body)
	if err != nil {
		log.Printf("%s.Handler - unmarhsal body error: %+v", handler, err)
	}
	query, _ := url.ParseQuery(form.RawQuery)
	payload := query["payload"][0]
	request := Request{}
	err = json.Unmarshal([]byte(payload), &request)
	if err != nil {
		log.Printf("%s.Handler - unmarhsal payload error: %+v", handler, err)
	}
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

	defer createIssue(request)

	err = request.PutItem()
	log.Printf("%s.Handler - submitted: %+v, error: %v", handler, request, err)

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

func createIssue(request Request) {
	bug := request.ToBug()

	jiraURL := fmt.Sprintf(jiraHost, os.Getenv("JIRA_API_HOST"))
	jiraUser := os.Getenv("JIRA_API_USER")
	jiraToken := os.Getenv("JIRA_API_TOKEN")

	inputQueue := map[string]interface{}{
		"fields": map[string]interface{}{
			"project":     map[string]string{"key": "IQ"},
			"summary":     bug.Summary,
			"description": fmt.Sprintf("Product: %s\nReporter: %s\n\n%s", bug.ProductName(), bug.UserName, bug.Details),
			"issuetype":   map[string]string{"name": "Bug"},
			"labels":      []string{"slack"},
			"priority":    map[string]string{"name": "Not Yet Prioritized"},
		},
	}

	iq, err := json.Marshal(inputQueue)
	log.Printf("%s.Handler - inputQueue: %+v, error: %v", handler, inputQueue, err)
	if err != nil {
		return
	}

	r, err := http.NewRequest("POST", jiraURL, bytes.NewBuffer(iq))
	if err != nil {
		log.Printf("%s.Handler - newRequest: %+v, error: %v", handler, inputQueue, err)
		return
	}
	r.SetBasicAuth(jiraUser, jiraToken)
	r.Header.Set("Content-Type", "application/json")
	c := &http.Client{}
	rr, err := c.Do(r)
	if err != nil {
		log.Printf("%s.Handler - createIssue: %+v, error: %v", handler, inputQueue, err)
		return
	}

	var issue struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Self string `json:"self"`
	}
	err = json.NewDecoder(rr.Body).Decode(&issue)
	log.Printf("%s.Handler - issue: %+v, error: %v", handler, issue, err)
	if err != nil {
		return
	}
	defer rr.Body.Close()

	payload, _ := json.Marshal(map[string]interface{}{
		"text": fmt.Sprintf("Bug submitted - ID: %s, Key: %s, Issue Link: %s",
			issue.ID, issue.Key, fmt.Sprintf("https://%s/projects/IQ/issues/%s", os.Getenv("JIRA_API_HOST"), issue.Key)),
	})
	req, reqErr := http.NewRequest("POST", request.ResponseURL, bytes.NewBuffer(payload))
	if reqErr != nil {
		log.Printf("%s.Handler - error sending dialog response url: %v", handler, reqErr)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_ACCESS_TOKEN"))
	client := &http.Client{}
	resp, respErr := client.Do(req)
	if respErr != nil {
		log.Printf("%s.Handler - error receiving dialog response from response url: %v", handler, reqErr)
		return
	}
	var respBody map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&respBody)
	log.Printf("%s.Handler - error receiving dialog response Body: %v", handler, respBody)
	defer resp.Body.Close()
}

func main() {
	lambda.Start(Handler)
}
