package v2

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/VirgilZhao/mailtohttp/model"
	"github.com/emersion/go-imap/client"
	"log"
	"time"
)

type App struct {
	config        *model.ServiceConfig
	client        *client.Client
	msgChan       chan string
	Name          string
	IsInLoginLoop bool
	stopLoginChan chan string
}

func (a *App) sendMessage(method string, text string) {
	log.Println(a.Name + "-" + method + ":" + text)
	data := model.SocketMessage{
		MsgType: "message",
		Data:    a.Name + "-" + method + ":" + text,
	}
	bytes, err := json.Marshal(&data)
	if err != nil {
		return
	}
	a.msgChan <- string(bytes)
}

func (a *App) login() error {
	a.sendMessage("login", "Connecting to server ...")
	t := time.NewTicker(5 * time.Second)
LOOP:
	for {
		a.IsInLoginLoop = true
		select {
		case <-t.C:
			c, err := client.DialTLS(fmt.Sprintf("%s:%v", a.config.EmailSettings.ImapAddress, a.config.EmailSettings.ImapPort), nil)
			if err != nil {
				a.sendMessage("login", err.Error())
				break
			}
			a.client = c
			a.sendMessage("login", "Connected")

			if err := c.Login(a.config.EmailSettings.Email, a.config.EmailSettings.Password); err != nil {
				a.sendMessage("login", err.Error())
				break
			}
			a.sendMessage("login", "Logged in")
			break LOOP
		case <-a.stopLoginChan:
			a.sendMessage("login", "stop by signal")
			a.IsInLoginLoop = false
			return errors.New("stop by signal")
		}
	}
	a.IsInLoginLoop = false
	return nil
}
