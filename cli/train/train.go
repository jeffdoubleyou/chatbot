package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/jeffdoubleyou/chatbot/bot"
	"github.com/jeffdoubleyou/chatbot/bot/adapters/storage"
)

var (
	dir      = flag.String("d", "/Users/jeffreyweitz/src/chatterbot-corpus/chatterbot_corpus/data/english", "the directory to look for corpora files")
	sqliteDB = flag.String("sqlite3", "/Users/jeffreyweitz/src/chatbot/chatbot.db", "the file path of the corpus sqlite3")
	//sqliteDB      = flag.String("sqlite3", "", "the file path of the corpus sqlite3")
	project       = flag.String("project", "DMS", "the name of the project in sqlite3 db")
	corpora       = flag.String("i", "", "the corpora files, comma to separate multiple files")
	storeFile     = flag.String("o", "corpus.gob", "the file to store corpora")
	printMemStats = flag.Bool("m", false, "enable printing memory stats")
)

func main() {
	flag.Parse()

	var files []string
	if len(*dir) > 0 {
		files = findCorporaFiles(*dir)
	}

	var corporaFiles string
	if len(files) > 0 {
		corporaFiles = strings.Join(files, ",")
		if len(*corpora) > 0 {
			corporaFiles = strings.Join([]string{corporaFiles, *corpora}, ",")
		}
	} else {
		corporaFiles = *corpora
	}

	if len(corporaFiles) == 0 && *sqliteDB == "" {
		flag.Usage()
		return
	}

	store, err := storage.NewSeparatedMemoryStorage(*storeFile)
	if err != nil {
		log.Fatal(err)
	}

	chatbot := &bot.ChatBot{
		PrintMemStats:  *printMemStats,
		Trainer:        bot.NewCorpusTrainer(store),
		StorageAdapter: store,
		Config: bot.Config{
			Project:    *project,
			Driver:     "sqlite3",
			DataSource: "chatbot.db",
		},
	}
	if len(strings.Split(corporaFiles, ",")) > 0 {
		corpuses, err := chatbot.LoadCorpusFromFiles(strings.Split(corporaFiles, ","))
		if err == nil {
			chatbot.SaveCorpusToDB(corpuses)
		}
	}
	if *sqliteDB != "" {
		if err := chatbot.TrainWithDB(); err != nil {
			log.Fatal(err)
		}
	} else {
		if err := chatbot.Train(strings.Split(corporaFiles, ",")); err != nil {
			log.Fatal(err)
		}
	}
}

func findCorporaFiles(dir string) []string {
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
	} else {
		fmt.Printf("Got files...")
	}

	return append(files, yamlFiles...)
}
