package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/jeffdoubleyou/chatbot/bot"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
)

type ProjectService service
type CorpusService service
type ResponseService service

type service struct {
	client *Client
}

type Response struct {
	*http.Response
	message *Message
}

type Message struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Error   string `json:"error"`
	Url     string `json:"url"`
}

type Client struct {
	client *http.Client
	apiUrl *url.URL
	Debug  bool

	common service

	Project  *ProjectService
	Corpus   *CorpusService
	Response *ResponseService
}

func NewClient(host string, port int) *Client {
	apiUrl, _ := url.Parse(fmt.Sprintf("%s:%d", host, port))
	client := &Client{
		client: &http.Client{},
		apiUrl: apiUrl,
	}
	client.common.client = client
	client.Project = (*ProjectService)(&client.common)
	client.Corpus = (*CorpusService)(&client.common)
	client.Response = (*ResponseService)(&client.common)

	return client
}

func (client *Client) SetDebug(debug bool) {
	client.Debug = debug
}

func (client *Client) ParseUrl(uri string) *url.URL {
	url, _ := client.apiUrl.Parse(uri)
	return url
}

func newResponse(r *http.Response) *Response {
	return &Response{r, nil}
}

func (res *Response) dumpResponse() {
	if dump, err := httputil.DumpResponse(res.Response, true); err != nil {
		fmt.Printf("could not dump response: %s\n", err.Error())
	} else {
		fmt.Printf("server response: %s\n", dump)
	}
}

func (res *Response) dumpRequest() {
	if dump, err := httputil.DumpRequest(res.Request, true); err != nil {
		fmt.Printf("could not dump request: %s\n", err.Error())
	} else {
		fmt.Printf("client request: %s\n", dump)
	}
}

func (client *Client) NewRequest(method, urlStr string, body interface{}) (*http.Request, error) {

	var buf io.ReadWriter

	var queryString *url.Values
	if body != nil {
		if method == "GET" {
			q := url.Values{}
			for key, val := range body.(map[string]interface{}) {
				q.Add(key, fmt.Sprintf("%v", val))
			}
			queryString = &q
			body = nil
		} else {
			buf = &bytes.Buffer{}
			enc := json.NewEncoder(buf)
			enc.SetEscapeHTML(false)
			err := enc.Encode(body)
			if err != nil {
				return nil, err
			}
		}
	}

	req, err := http.NewRequest(method, urlStr, buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if queryString != nil {
		req.URL.RawQuery = queryString.Encode()
	}

	req.Header.Set("Accept", "application/json")

	return req, nil
}

func (client *Client) Do(r *http.Request, v interface{}) (*Response, error) {
	resp, err := client.client.Do(r)
	if err != nil {
		return nil, err
	}

	response := newResponse(resp)

	defer resp.Body.Close()

	if client.Debug {
		response.dumpRequest()
		response.dumpResponse()
	}

	if resp.StatusCode != 200 {
		//fmt.Printf("Request to %s failed with code %d", resp.Request.URL, resp.StatusCode)
		message := &Message{}
		response.message = message
		if err := json.NewDecoder(resp.Body).Decode(message); err != nil {
			//fmt.Printf("Could not parse error response")
			return response, fmt.Errorf("request failed with code %d", resp.StatusCode)
		} else {
			if message.Message != "" {
				return response, fmt.Errorf("%s", message.Message)
			} else {
				return response, fmt.Errorf("request failed with code %d and the error message was empty", resp.StatusCode)
			}
		}

	}

	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return response, err
	}

	// This is to handle the weird "message": "OK" return types for some APIs
	switch v.(type) {
	case Message:
		if v.(Message).Message == "OK" {
			return response, nil
		} else {
			return response, fmt.Errorf("error: %s", v.(Message).Error)
		}
	default:
		return response, nil
	}
}

func (project ProjectService) GetProjectList() ([]*bot.Project, error) {
	url := project.client.ParseUrl("project/")
	req, err := project.client.NewRequest("GET", url.String(), nil)
	if err != nil {
		return nil, err
	}
	var projects []*bot.Project
	if _, err = project.client.Do(req, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (project ProjectService) AddProject(name, config string) (*bot.Project, error) {
	p := &bot.Project{
		Name:   name,
		Config: config,
	}
	url := project.client.ParseUrl("project/")
	if req, err := project.client.NewRequest("POST", url.String(), p); err != nil {
		return nil, err
	} else {
		if _, err = project.client.Do(req, &p); err != nil {
			return nil, err
		} else {
			return p, nil
		}
	}
}

type Corpus struct {
	bot.Corpus
	Score float32 `json:"score"`
}

type Responses struct {
	Question string    `json:"question"`
	Results  []*Corpus `json:"results"`
	Message  string    `json:"message"`
}

func (response *ResponseService) GetResponse(project, query, context string) (responses *Responses, err error) {
	url := response.client.ParseUrl("respond/" + project)
	args := map[string]interface{}{
		"q":       query,
		"context": context,
	}
	if req, err := response.client.NewRequest("GET", url.String(), args); err != nil {
		return nil, err
	} else {
		if _, err = response.client.Do(req, &responses); err != nil {
			return nil, err
		} else {
			return responses, nil
		}
	}
}
