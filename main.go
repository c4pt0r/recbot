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
	"database/sql"
	"encoding/json"
	"flag"

	"github.com/c4pt0r/log"
	_ "github.com/go-sql-driver/mysql"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var (
	token   = flag.String("tgbot-token", "", "Telegram bot token")
	debug   = flag.Bool("debug", false, "Enable debug mode")
	tidbDSN = flag.String("tidb-dsn", "root@tcp(", "TiDB DSN")
)

var db *sql.DB

func initDB() {
	var err error
	db, err = sql.Open("mysql", *tidbDSN)
	if err != nil {
		log.Fatal(err)
	}

	// create table if not exists
	stmt := `CREATE TABLE IF NOT EXISTS recbot (
		id INT NOT NULL AUTO_INCREMENT,
		content JSON NOT NULL,
		create_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY(create_at),
		PRIMARY KEY (id))
		`
	_, err = db.Exec(stmt)
	if err != nil {
		log.Fatal(err)
	}
}

func insertMessage(msg *tgbotapi.Message) error {
	b, _ := json.Marshal(msg)
	stmt := `INSERT INTO recbot (content) VALUES (?)`
	_, err := db.Exec(stmt, b)
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
		err := insertMessage(update.Message)
		if err != nil {
			log.Error(err)
		}
		bot.Send(tgbotapi.NewMessage(update.Message.Chat.ID, "ACK"))
	}
}
