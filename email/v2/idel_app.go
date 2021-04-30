package v2

import (
	"fmt"
	"github.com/VirgilZhao/mailtohttp/model"
	idle "github.com/emersion/go-imap-idle"
	"github.com/emersion/go-imap/client"
	"time"
)

type IdleApp struct {
	App
	idleClient   *idle.IdleClient
	stopChan     chan string
	MessageCount uint32
}

func NewIdleApp(config *model.ServiceConfig, msgChan chan string) *IdleApp {
	return &IdleApp{
		App: App{
			Name:          "IdleApp",
			config:        config,
			msgChan:       msgChan,
			stopLoginChan: make(chan string, 1),
		},
		stopChan:     make(chan string, 1),
		MessageCount: 0,
	}
}

func (ia *IdleApp) Start(updateNotifyChan chan string) {
START:
	if err := ia.login(); err != nil {
		ia.sendMessage("Start", "login error:"+err.Error())
		return
	}
	defer ia.client.Logout()
	mbox, err := ia.client.Select(ia.config.EmailSettings.Folder, false)
	if err != nil {
		ia.sendMessage("Start", err.Error())
		return
	}
	ia.sendMessage("Start", fmt.Sprintf("mailbox %s with flags %v", mbox.Name, mbox.Flags))
	ia.idleClient = idle.NewClient(ia.client)
	updates := make(chan client.Update)
	ia.client.Updates = updates
	done := make(chan error, 1)
	stop := make(chan struct{})
	go func() {
		done <- ia.idleClient.IdleWithFallback(stop, 1*time.Minute)
	}()
	for {
		ia.sendMessage("Start", "listen updates")
		select {
		case update := <-updates:
			ia.sendMessage("Start", "new update")
			switch update.(type) {
			case *client.MailboxUpdate:
				mailbox := update.(*client.MailboxUpdate)
				ia.sendMessage("start", fmt.Sprintf("mailbox update found with total %d message", mailbox.Mailbox.Messages))
				if ia.MessageCount != mailbox.Mailbox.Messages {
					updateNotifyChan <- mailbox.Mailbox.Name
					ia.MessageCount = mailbox.Mailbox.Messages
				}
				break
			default:
				break
			}
		case err := <-done:
			ia.sendMessage("start", "error:"+err.Error())
			ia.sendMessage("start", "not idling")
			goto START
		case <-ia.stopChan:
			ia.sendMessage("start", "quit by stop signal")
			return
		}
	}
}

func (ia *IdleApp) Stop() {
	if ia.IsInLoginLoop {
		ia.sendMessage("Stop", "stop login")
		ia.stopLoginChan <- ""
	} else {
		ia.sendMessage("stop", "stop idle")
		ia.stopChan <- ""
	}
}
