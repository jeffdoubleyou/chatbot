package bot

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/go-xorm/xorm"
	"github.com/jeffdoubleyou/chatbot/bot/corpus"
	"github.com/prometheus/common/log"

	"runtime"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jeffdoubleyou/chatbot/bot/adapters/logic"
	"github.com/jeffdoubleyou/chatbot/bot/adapters/storage"
	_ "github.com/mattn/go-sqlite3"
)

const mega = 1024 * 1024

type ChatBot struct {
	PrintMemStats bool
	//InputAdapter   input.InputAdapter
	LogicAdapter logic.LogicAdapter
	//OutputAdapter  output.OutputAdapter
	StorageAdapter storage.StorageAdapter
	Trainer        Trainer
	Config         Config
}

type CORPUS_TYPE int

const (
	CORPUS_CORPUS      CORPUS_TYPE = 1
	CORPUS_REQUIREMENT CORPUS_TYPE = 2
)

type ChatBotFactory struct {
	mu       sync.Mutex
	chatBots map[string]*ChatBot
	config   Config
}

//var chatBotFactory *ChatBotFactory

func NewChatBotFactory(config Config) *ChatBotFactory {

	return &ChatBotFactory{
		mu:       sync.Mutex{},
		config:   config,
		chatBots: make(map[string]*ChatBot),
	}

}
func (f *ChatBotFactory) Init() {
	fmt.Printf("Initialize factory....\n")
	var err error
	if engine == nil {
		fmt.Printf("New Engine using %s from %s\n", f.config.Driver, f.config.DataSource)
		engine, err = xorm.NewEngine(f.config.Driver, f.config.DataSource)
		if err != nil {
			panic(err)
		}
		err = engine.Sync2(&Corpus{}, &Project{}, &Feedback{})
		if err != nil {
			log.Error(err)
		}
	}
	projects := make([]Project, 0)
	err = engine.Find(&projects)
	for _, project := range projects {
		fmt.Printf("Initializing project %s\n", project.Name)
		var conf Config
		json.Unmarshal([]byte(project.Config), &conf)
		if project.Name == "" {
			continue
		}
		conf.Project = project.Name
		fmt.Printf("Loading project '%s'\n", project.Name)
		if _, ok := f.GetChatBot(project.Name); !ok {
			store := storage.NewMemoryStorage()
			chatbot := &ChatBot{
				LogicAdapter:   logic.NewClosestMatch(store, 5),
				PrintMemStats:  f.config.PrintMemStats,
				Trainer:        NewCorpusTrainer(store),
				StorageAdapter: store,
				Config:         conf,
			}
			f.AddChatBot(project.Name, chatbot)
			chatbot.Init()
		}
	}

}

func (f *ChatBotFactory) Refresh() {
	f.Init()
}

func (f *ChatBotFactory) GetChatBot(project string) (*ChatBot, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	chatBot, ok := f.chatBots[project]
	return chatBot, ok
}

func (f *ChatBotFactory) AddChatBot(project string, chatBot *ChatBot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.chatBots[project]; !ok {
		f.chatBots[project] = chatBot
	}

}

func (f *ChatBotFactory) ListProject() []Project {
	var projects []Project
	var err error
	err = engine.Find(&projects)
	if err != nil {
		log.Error(err)
	}
	return projects
}

func (f *ChatBotFactory) AddProject(name, config string) (*Project, error) {
	project := &Project{
		Name:   name,
		Config: config,
	}

	if exists, _ := f.GetProject(name); exists {
		return nil, fmt.Errorf("project with name '%s' already exists", name)
	}

	if id, err := engine.Insert(project); err != nil {
		return nil, err
	} else {
		fmt.Printf("Added new project %s with ID %d\n", name, id)
		project.Id = int(id)
		return project, nil
	}
}

func (f *ChatBotFactory) GetProject(name string) (bool, error) {
	project := &Project{
		Name: name,
	}
	if p, err := engine.Get(project); err != nil {
		return false, err
	} else {
		return p, nil
	}
}

func (f *ChatBotFactory) GetCorpusById(id int) *Corpus {
	var corpus []Corpus
	session := engine.Limit(1, 0).Where("id = ?", id)
	if err := session.Find(&corpus); err != nil {
		fmt.Printf("Error retrieving corpus ID %d: %s\n", id, err.Error())
		return nil
	} else {
		if len(corpus) == 1 {
			return &corpus[0]
		} else {
			fmt.Printf("Could not find corpus ID %d", id)
			return nil
		}
	}
}

func (f *ChatBotFactory) ListCorpus(corpus Corpus, start int, limit int) []Corpus {
	var corpuses []Corpus
	var err error
	session := engine.Limit(limit, start)
	if corpus.Project != "" {
		session.Where("project = ?", corpus.Project)
	}
	if corpus.Question != "" {
		session.Where("question like ?", "%"+corpus.Question+"%")
	}
	err = session.Find(&corpuses)
	if err != nil {
		log.Error(err)
	}
	return corpuses
}

var engine *xorm.Engine

func (chatbot *ChatBot) Init() {
	var err error
	if engine == nil {
		engine, err = xorm.NewEngine(chatbot.Config.Driver, chatbot.Config.DataSource)
	}
	if err != nil {
		panic(err)
	}

	err = engine.Sync2(&Corpus{}, &Project{}, &Feedback{})
	if err != nil {
		log.Error(err)
	}

	if chatbot.Config.DirCorpus != "" {
		files := chatbot.FindCorporaFiles(chatbot.Config.DirCorpus)
		if len(files) > 0 {
			corpuses, _ := chatbot.LoadCorpusFromFiles(files)
			if len(corpuses) > 0 {
				chatbot.SaveCorpusToDB(corpuses)

			}
		}
	}
	err = chatbot.TrainWithDB()
	if err != nil {
		log.Error(err)
		panic(err)
	}
}

type Corpus struct {
	Id       int    `json:"id" form:"id" xorm:"int pk autoincr notnull 'id' comment('编号')"`
	Class    string `json:"class" form:"class"  xorm:"varchar(255) notnull 'class' comment('分类')"`
	Project  string `json:"project" form:"project" xorm:"varchar(255) notnull 'project' comment('项目')"`
	Question string `json:"question" form:"question"  xorm:"varchar(2048) notnull  'question' comment('问题')"`
	Answer   string `json:"answer" form:"answer" xorm:"text notnull  'answer' comment('回答')"`
	//Creator     string    `json:"creator" form:"creator" xorm:"varchar(256) notnull  'creator' comment('创建人')"`
	Principal   string     `json:"principal" form:"principal" xorm:"varchar(256) notnull  'principal' comment('责负人')"`
	Reviser     string     `json:"reviser" form:"reviser" xorm:"varchar(256) notnull  'reviser' comment('修订人')"`
	AcceptCount int        `json:"accept_count" form:"accept_count" xorm:"int notnull default 0  'accept_count' comment('解决次数')"`
	RejectCount int        `json:"reject_count" form:"reject_count" xorm:"int notnull  default 0 'reject_count' comment('解决次数')"`
	CreatTime   time.Time  `json:"creat_time" xorm:"creat_time created" json:"creat_time" description:"创建时间"`
	UpdateTime  time.Time  `json:"update_time" xorm:"update_time updated"json:"update_time"description:"更新时间"`
	Qtype       int        `json:"qtype" form:"qtype" xorm:"int notnull 'qtype' comment('类型，需求，问答')"`
	Context     string     `json:"context" form:"context" xorm:"varchar(255) notnull default '' 'context' comment('Context after answer')"`
	Contextual  bool       `json:"contextual" form:"contextual" xorm:"int(1) not null default 0 'contextual' comment('Is this conversation contextual')"`
	Data        CorpusData `json:"data" form:"data" xorm:"text notnull default '' 'data' comment('Data')"`
}

type CorpusData struct {
	Data map[string]interface{}
}

func (corpusData *CorpusData) FromDB(data []byte) error {
	if data == nil || len(data) == 0 {
		corpusData = &CorpusData{}
		return nil
	} else {
		return json.Unmarshal(data, corpusData)
	}
}

func (corpusData *CorpusData) ToDB() ([]byte, error) {
	fmt.Printf("Corpus data to DB")
	if corpus, err := json.Marshal(corpusData); err != nil {
		return nil, err
	} else {
		return corpus, nil
	}
}

type Feedback struct {
	Id          int       `json:"id" form:"id" xorm:"int pk autoincr notnull 'id' comment('编号')"`
	Cid         int       `json:"cid" form:"cid" xorm:"int  notnull 'cid' comment('语料编号')"`
	Class       string    `json:"class" form:"class"  xorm:"varchar(255) notnull 'class' comment('分类')"`
	Project     string    `json:"project" form:"project" xorm:"varchar(255) notnull 'project' comment('项目')"`
	Question    string    `json:"question" form:"question"  xorm:"varchar(2048) notnull  'question' comment('问题')"`
	Answer      string    `json:"answer" form:"answer" xorm:"text notnull   'answer' comment('回答')"`
	Creator     string    `json:"creator" form:"creator" xorm:"varchar(256) notnull  'creator' comment('创建人')"`
	Principal   string    `json:"principal" form:"principal" xorm:"varchar(256) notnull  'principal' comment('责负人')"`
	Reviser     string    `json:"reviser" form:"reviser" xorm:"varchar(256) notnull  'reviser' comment('修订人')"`
	AcceptCount int       `json:"accept_count" form:"accept_count" xorm:"int notnull default 0  'accept_count' comment('解决次数')"`
	RejectCount int       `json:"reject_count" form:"reject_count" xorm:"int notnull default 0  'reject_count' comment('解决次数')"`
	CreatTime   time.Time `json:"creat_time" xorm:"creat_time created" json:"creat_time" description:"创建时间"`
	UpdateTime  time.Time `json:"update_time" xorm:"update_time updated"json:"update_time"description:"更新时间"`
	Qtype       int       `json:"qtype" form:"qtype" xorm:"int notnull 'qtype' comment('类型，需求，问答')"`
}

type Project struct {
	Id     int    `json:"id" form:"id" xorm:"int pk autoincr notnull 'id' comment('编号')"`
	Name   string `json:"name" form:"name"  xorm:"varchar(255) notnull 'name' comment('名称')"`
	Config string `json:"config" form:"config"  xorm:"text notnull 'config' comment('配置')"`
	//Config Config
}

type Config struct {
	Driver        string `json:"driver"`
	DataSource    string `json:"data_source"`
	Project       string `json:"project"`
	DirCorpus     string `json:"dir_corpus"`
	StoreFile     string `json:"store_file"`
	PrintMemStats bool   `json:"print_mem_stats"`
}

func (chatbot *ChatBot) Train(data interface{}) error {
	start := time.Now()
	defer func() {
		fmt.Printf("Elapsed: %s\n", time.Since(start))
	}()

	if chatbot.PrintMemStats {
		go func() {
			for {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fmt.Printf("Alloc = %vm\nTotalAlloc = %vm\nSys = %vm\nNumGC = %v\n\n",
					m.Alloc/mega, m.TotalAlloc/mega, m.Sys/mega, m.NumGC)
				time.Sleep(5 * time.Second)
			}
		}()
	}

	if err := chatbot.Trainer.Train(data); err != nil {
		fmt.Printf("Failed to train data: %s\n", err.Error())
		return err
	} else {
		fmt.Printf("Training complete for chatbot %s\n", chatbot.Config.Project)
		return chatbot.StorageAdapter.Sync()
	}
}

func (chatbot *ChatBot) LoadCorpusFromDB() (map[string][][]string, error) {
	results := make(map[string][][]string)
	var rows []Corpus
	query := Corpus{
		Project: chatbot.Config.Project,
		Qtype:   int(CORPUS_CORPUS),
	}
	err := engine.Find(&rows, &query)
	if err != nil {
		return nil, err
	}
	var corpuses [][]string
	exp, err := regexp.Compile(`[|｜\r\n]+`)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		var corpus []string
		questions := exp.Split(row.Question, -1)

		for _, question := range questions {
			if strings.TrimSpace(question) == "" {
				continue
			}
			if !strings.HasSuffix(question, "?") && !strings.HasSuffix(question, "？") {
				question = question + "?"
			}
			corpus = append(corpus, question, fmt.Sprintf("%s$$$$%s$$$$%v$$$$%s", question, row.Answer, row.Id, row.Context))
		}
		corpuses = append(corpuses, corpus)
	}
	results[chatbot.Config.Project] = corpuses
	return results, nil

}

func (chatbot *ChatBot) LoadCorpusFromFiles(filePaths []string) (map[string][][]string, error) {
	return corpus.LoadCorpora(filePaths)
}

func (chatbot *ChatBot) SaveCorpusToDB(corpuses map[string][][]string) {
	for k, v := range corpuses {
		for _, cp := range v {
			if len(cp) == 2 {
				corpus := Corpus{
					Class:    k,
					Question: cp[0],
					Answer:   cp[1],
					Qtype:    1,
					Project:  chatbot.Config.Project,
				}
				chatbot.AddCorpusToDB(&corpus)
			}
		}
	}

}

func (chatbot *ChatBot) AddFeedbackToDB(feedback *Feedback) error {
	corpus := Corpus{
		Id: feedback.Cid,
	}
	var (
		ok  bool
		err error
	)
	if ok, _ = engine.Get(&corpus); ok {
		feedback.Project = corpus.Project
		feedback.Class = corpus.Class
		_, err = engine.Insert(feedback)
		return err
	} else {
		_, err = engine.Insert(feedback)
	}

	return err

}

func (chatbot *ChatBotFactory) UpdateCorpusCounter(id int, isOk bool) error {
	if id <= 0 {
		return fmt.Errorf("%v", "编号<0不合法")
	}
	q := Corpus{
		Id: id,
	}
	var (
		err error
		ok  bool
	)
	if ok, err = engine.Get(&q); ok {
		if isOk {
			q.AcceptCount = q.AcceptCount + 1
		} else {
			q.RejectCount = q.RejectCount + 1
		}
		if id > 0 {
			_, err = engine.Id(id).Cols("reject_count", "accept_count").Update(&q)
		}
	} else {
		err = fmt.Errorf("record not found")
	}
	return err

}

func (chatbot *ChatBot) AddCorpusToDB(corpus *Corpus) error {
	q := Corpus{
		Question: corpus.Question,
		Class:    corpus.Class,
	}
	if corpus.Id != 0 {
		q = Corpus{
			Id: corpus.Id,
		}
	}

	if ok, err := engine.Get(&q); !ok {
		_, err = engine.Insert(corpus)
		return err
	} else {
		if q.Id > 0 {
			corpus.Id = q.Id
			_, err = engine.Update(corpus, &Corpus{Id: q.Id})
			return err
		}
	}
	return nil
}

func (chatbot *ChatBot) RemoveCorpusFromDB(corpus *Corpus) error {
	q := Corpus{}
	if corpus.Id > 0 {
		q.Id = corpus.Id
	} else {
		if corpus.Question == "" {
			return errors.New("id or question must be set value")
		}
		q.Question = corpus.Question
	}
	if ok, err := engine.Get(&q); ok {
		chatbot.StorageAdapter.Remove(q.Question)
		engine.Delete(&q)
		return err
	}
	return nil
}

func (chatbot *ChatBot) TrainWithDB() error {
	start := time.Now()
	defer func() {
		fmt.Printf("Elapsed: %s\n", time.Since(start))
	}()

	if chatbot.PrintMemStats {
		go func() {
			for {
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				fmt.Printf("Alloc = %vm\nTotalAlloc = %vm\nSys = %vm\nNumGC = %v\n\n",
					m.Alloc/mega, m.TotalAlloc/mega, m.Sys/mega, m.NumGC)
				time.Sleep(5 * time.Second)
			}
		}()
	}

	corpuses, err := chatbot.LoadCorpusFromDB()
	if err != nil {
		return err
	}

	if err := chatbot.Trainer.TrainWithCorpus(corpuses); err != nil {
		return err
	} else {
		return nil
		//return chatbot.StorageAdapter.Sync()
	}

}

func (chatbot *ChatBot) FindCorporaFiles(dir string) []string {
	var files []string

	jsonFiles, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		fmt.Println(err)
		return nil
	}

	files = append(files, jsonFiles...)

	ymlFiles, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		fmt.Println(err)
		return nil
	}

	files = append(files, ymlFiles...)

	yamlFiles, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		fmt.Println(err)
		return nil
	}

	return append(files, yamlFiles...)
}

func (chatbot *ChatBot) GetResponse(text string, context ...string) []logic.Answer {
	if chatbot.LogicAdapter.CanProcess(text) {
		return chatbot.LogicAdapter.Process(text, context...)
	}
	return nil
}
