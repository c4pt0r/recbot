// recbot
// Copyright (C) mememe author 2022
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package main

import (
	"bytes"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/c4pt0r/log"
	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	token   = flag.String("tgbot-token", "", "Telegram bot token")
	debug   = flag.Bool("debug", false, "Enable debug mode")
	tidbDSN = flag.String("tidb-dsn", "root@tcp(", "TiDB DSN")
	flushDB = flag.Bool("flush-db", false, "flush database")
)

var (
	summaryCommand string = "curl -s %s | strip-tags | ttok -t 4000  | llm --system '用中文总结，并将总结的内容以要点列表返回'"

	askGptCommand string = "llm"
)

var db *sql.DB

func createTables(db *sql.DB) error {
	// create table if not exists
	stmt := `CREATE TABLE IF NOT EXISTS recbot (
		id INT NOT NULL AUTO_INCREMENT,
		chat_id BIGINT NOT NULL,
		content JSON NOT NULL, 
		reply BLOB DEFAULT NULL,
		create_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY(create_at),
		PRIMARY KEY (id))
		`
	_, err := db.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func initDB() {
	var err error
	mysql.RegisterTLSConfig("tidb", &tls.Config{
		MinVersion: tls.VersionTLS12,
		// FIXME
		ServerName: "gateway01.us-west-2.prod.aws.tidbcloud.com",
	})

	db, err = sql.Open("mysql", *tidbDSN)
	if err != nil {
		log.Fatal(err)
	}

	if *flushDB {
		_, err = db.Exec("DROP TABLE IF EXISTS recbot")
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("your call")
		os.Exit(0)
	} else {
		createTables(db)
	}
}

func executeOsCommand(command string, stdinContent []byte) (string, error) {
	// run shell command and return the stdout
	cmd := exec.Command("bash", "-c", command)
	// set exec os env
	cmd.Env = os.Environ()
	var out bytes.Buffer
	if stdinContent != nil {
		cmd.Stdin = bytes.NewReader(stdinContent)
	}
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

func insertMessage(msg *tgbotapi.Message) (int64, error) {
	b, _ := json.Marshal(msg)
	stmt := `INSERT INTO recbot (content, chat_id) VALUES (?, ?)`
	ret, err := db.Exec(stmt, b, msg.Chat.ID)
	if err != nil {
		return -1, err
	}
	lastInsertID, err := ret.LastInsertId()
	if err != nil {
		return -1, err
	}
	return lastInsertID, err
}

func updateMessage(id int64, reply string) error {
	stmt := `UPDATE recbot SET reply = ? WHERE id = ?`
	_, err := db.Exec(stmt, reply, id)
	return err
}

func isMention(update *tgbotapi.Update, who string) bool {
	for _, e := range update.Message.Entities {
		if e.Type == "mention" {
			name := update.Message.Text[e.Offset+1 : e.Offset+e.Length]
			if name == who {
				return true
			}
		}
	}
	return false
}

func isPrivateMessage(update *tgbotapi.Update) bool {
	return update.Message.Chat.Type == "private"
}

func isURL(msg string) bool {
	if strings.HasPrefix(msg, "http://") || strings.HasPrefix(msg, "https://") {
		return true
	}
	return false
}

func main() {
	flag.Parse()

	if *token == "" {
		log.Fatal("Telegram bot token is required")
	}

	if *tidbDSN == "" {
		log.Fatal("TiDB DSN is required")
	}

	initDB()

	bot, err := tgbotapi.NewBotAPI(*token)
	if err != nil {
		log.Fatal(err)
	}

	bot.Debug = *debug
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	me, err := bot.GetMe()
	if err != nil {
		log.Fatal(err)
	}

	updates := bot.GetUpdatesChan(updateConfig)
	for update := range updates {
		if (update.Message == nil) ||
			(!isPrivateMessage(&update) && !isMention(&update, me.String())) {
			continue
		}
		msgID, err := insertMessage(update.Message)
		if err != nil {
			log.Error(err)
		}
		// DO your work
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "Recieved...and thinking..."))
		go func(update tgbotapi.Update, msgID int64) {
			var out string
			var err error
			if isURL(update.Message.Text) {
				out, err = executeOsCommand(fmt.Sprintf(summaryCommand, update.Message.Text), nil)
			} else {
				out, err = executeOsCommand(askGptCommand, []byte(update.Message.Text))
			}
			if err != nil {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, err.Error()))
			} else {
				bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, out))
				updateMessage(msgID, out)
			}
		}(update, msgID)

	}
}
