package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/jeffdoubleyou/chatbot/bot"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var factory *bot.ChatBotFactory

var (
	verbose       = flag.Bool("verbose", false, "verbose mode")
	answers       = flag.Int("answers", 5, "the number of answers to return")
	driver        = flag.String("driver", "sqlite3", "database driver")
	datasource    = flag.String("datasource", "chatbot.db", "database path")
	listenAddr    = flag.String("listen", "127.0.0.1", "server listen address")
	listenPort    = flag.Int("port", 8080, "server listen port")
	printMemStats = flag.Bool("memstats", false, "enable printing memory statistics")
)

type JsonResult struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type QA struct {
	Question   string                 `json:"question"`
	Answer     string                 `json:"answer"`
	Score      float32                `json:"score"`
	Context    string                 `json:"context"`
	Contextual bool                   `json:"contextual"`
	Class      string                 `json:"class"'`
	Data       map[string]interface{} `json:"data"'`
	ID         int                    `json:"id"`
}

type Response struct {
	Question string `json:"question"`
	Results  []*QA  `json:"results"`
	Message  string `json:"message"`
}

type ResoveReq struct {
	IsOk bool `json:"is_ok"`
	Id   int  `json:"id"`
}

func init() {
	flag.Parse()
	config := bot.Config{
		Driver:        *driver,
		DataSource:    *datasource,
		Project:       "",
		DirCorpus:     "",
		StoreFile:     "",
		PrintMemStats: *printMemStats,
	}
	factory = bot.NewChatBotFactory(config)
	factory.Init()
}

func main() {
	router := mux.NewRouter()
	fs := http.FileServer(http.Dir("./docs/"))
	router.PathPrefix("/docs/").Handler(http.StripPrefix("/docs/", fs))

	// Projects
	project := router.PathPrefix("/project/").Subrouter()
	project.Path("/").Methods("GET").HandlerFunc(getProjectList)
	project.Path("/{project}").Methods("GET").HandlerFunc(getProject)
	project.Path("/").Methods("POST").HandlerFunc(addProject)
	project.Path("/{project}").Methods("DELETE").HandlerFunc(deleteProject)

	// Corpus
	corpus := router.PathPrefix("/corpus/").Subrouter()
	corpus.Path("/{project}").Methods("GET").HandlerFunc(listProjectCorpus)
	corpus.Path("/{project}").Methods("POST").HandlerFunc(addProjectCorpus)
	corpus.Path("/{project}/{id}").Methods("GET").HandlerFunc(getProjectCorpusById)
	corpus.Path("/{project}/{id}").Methods("DELETE").HandlerFunc(deleteProjectCorpus)
	corpus.Path("/{project}/{id}").Methods("PUT").HandlerFunc(updateProjectCorpus)

	respond := router.PathPrefix("/respond/").Subrouter()
	respond.Path("/{project}").Methods("GET").HandlerFunc(getResponse)
	respond.Path("/feedback/{project}").Methods("POST").HandlerFunc(addFeedback)

	serverAddress := fmt.Sprintf("%s:%d", *listenAddr, *listenPort)

	srv := &http.Server{
		Handler:      router,
		Addr:         serverAddress,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	fmt.Printf("Starting server on %s\n", serverAddress)
	srv.ListenAndServe()
}

func getProjectCorpusById(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	id, _ := strconv.Atoi(vars["id"])
	project := vars["project"]
	fmt.Printf("Get corpus ID %s from project %s\n", id, project)
	corp := factory.GetCorpusById(id)
	if corp != nil && corp.Project == project {
		SendJson(writer, corp)
	} else {
		SendError(writer, "Corpus not found", 404)
	}
}

func addFeedback(writer http.ResponseWriter, request *http.Request) {

}

func getResponse(writer http.ResponseWriter, request *http.Request) {
	request.ParseForm()
	vars := mux.Vars(request)
	project := vars["project"]

	query := request.Form.Get("q")
	context := request.Form.Get("context")

	response := &Response{
		Question: query,
		Results:  nil,
		Message:  "",
	}

	if query == "" {
		response.Message = "No query was provided"
		SendJson(writer, response)
		return
	}

	if bot, ok := factory.GetChatBot(project); !ok {
		response.Message = "Could not initialize project"
		SendJson(writer, response)
		return
	} else {
		var c []string
		if context != "" {
			c = []string{context}
		}
		answers := bot.GetResponse(query, c...)
		j, _ := json.MarshalIndent(answers, "", "\t")
		fmt.Printf("RES: %s\n", j)
		for _, answer := range answers {
			contents := strings.Split(answer.Content, "$$$$")
			if len(contents) > 2 {
				id, _ := strconv.Atoi(contents[2])
				corpus := factory.GetCorpusById(id)
				qa := &QA{
					Question:   contents[0],
					Answer:     contents[1],
					Score:      answer.Confidence,
					Context:    corpus.Context,
					Contextual: corpus.Contextual,
					Data:       corpus.Data.Data,
					Class:      corpus.Class,
					ID:         id,
				}
				response.Results = append(response.Results, qa)
			}
		}
		SendJson(writer, response)
		return
	}
}

func updateProjectCorpus(writer http.ResponseWriter, request *http.Request) {

}

func deleteProjectCorpus(writer http.ResponseWriter, request *http.Request) {

}

func addProjectCorpus(writer http.ResponseWriter, request *http.Request) {

}

func listProjectCorpus(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	project := vars["id"]
	c := bot.Corpus{
		Project: project,
	}
	request.ParseForm()
	start, _ := strconv.Atoi(request.Form.Get("start"))
	limit, _ := strconv.Atoi(request.Form.Get("limit"))
	if limit == 0 {
		limit = 100
	}
	corpora := factory.ListCorpus(c, start, limit)
	SendJson(writer, corpora)
}

func deleteProject(writer http.ResponseWriter, request *http.Request) {

}

func addProject(writer http.ResponseWriter, request *http.Request) {
	project := &bot.Project{}
	if err := ParseJsonBody(request, project); err != nil {
		SendError(writer, fmt.Sprintf("Could not parse request: %s", err.Error()))
	} else {
		if p, err := factory.AddProject(project.Name, project.Config); err != nil {
			SendError(writer, err.Error())
		} else {
			SendJson(writer, p)
		}
	}
}

func getProject(writer http.ResponseWriter, request *http.Request) {

}

func getProjectList(writer http.ResponseWriter, request *http.Request) {
	projects := factory.ListProject()
	SendJson(writer, projects)
}

func SendJson(w http.ResponseWriter, res interface{}, statusCode ...int) {
	r, _ := json.MarshalIndent(res, "", "\t")
	if len(statusCode) == 1 {
		w.WriteHeader(statusCode[0])
	} else {
		w.WriteHeader(200)
	}
	w.Write(r)
}

type Error struct {
	Message string
}

func SendError(w http.ResponseWriter, message string, statusCode ...int) {
	response := &Error{message}
	r, _ := json.MarshalIndent(response, "", "\t")
	if len(statusCode) == 1 {
		w.WriteHeader(statusCode[0])
	} else {
		w.WriteHeader(500)
	}
	w.Write(r)
}

func ParseJsonBody(r *http.Request, data interface{}) error {
	return json.NewDecoder(r.Body).Decode(data)
}
