package main

import (
	"embed"
	"encoding/json"
	"flag"
	"github.com/VirgilZhao/mailtohttp/email"
	"github.com/VirgilZhao/mailtohttp/model"
	"github.com/VirgilZhao/mailtohttp/utils"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

//go:embed app
var embedFiles embed.FS
var receiveApp *email.EmailApp
var idelApp *email.EmailApp
var status = "stopped"
var msgChan = make(chan string, 100)

const (
	configFileName = "config.mtt"
)

func getFileSystem(useOs bool) http.FileSystem {
	if useOs {
		log.Println("using live mode")
		return http.FS(os.DirFS("app"))
	}
	log.Println("using embed mode")
	fsys, err := fs.Sub(embedFiles, "app")
	if err != nil {
		panic(err)
	}
	return http.FS(fsys)
}

func loginHandler(c echo.Context) error {
	passwordCheck := c.Param("passText")
	login := "fail"
	if *password == passwordCheck {
		login = "ok"
	}
	config := loadConfig()
	sendMessage("status", status)
	return c.JSON(200, model.LoginResponse{
		Login:  login,
		Config: *config,
	})
}

func startServiceHandler(c echo.Context) error {
	config := model.ServiceConfig{
		ContentPatterns: make([]model.ServiceContentPattern, 0),
	}
	if err := c.Bind(&config); err != nil {
		return c.JSON(200, err.Error())
	}
	go startEmailLoop(&config)
	saveConfig(&config)
	return c.JSON(200, "ok")
}

func stopServiceHandler(c echo.Context) error {
	stopEmailLoop()
	status = "stopped"
	sendMessage("status", status)
	return c.JSON(200, "ok")
}

var logSocket = websocket.Upgrader{}
var conn *websocket.Conn

func webSocketHandler(c echo.Context) error {
	var err error
	if conn != nil {
		conn.Close()
	}
	conn, err = logSocket.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		panic(err)
	}
	go messageLoop()
	return nil
}

func messageLoop() {
	defer func() {
		if err := recover(); err != nil {
			log.Println(err)
			return
		}
	}()
	for {
		select {
		case msg := <-msgChan:
			err := conn.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				log.Println(err)
				return
			}
			break
		}
	}
}

func sendMessage(ctype, data string) {
	msg := model.SocketMessage{
		MsgType: ctype,
		Data:    data,
	}
	bytes, err := json.Marshal(&msg)
	if err != nil {
		log.Println(err)
		return
	}
	msgChan <- string(bytes)
}

func startEmailLoop(config *model.ServiceConfig) {
	if receiveApp != nil {
		receiveApp.StopLoop()
		receiveApp = nil
	}
	if idelApp != nil {
		idelApp.StopLoop()
		idelApp = nil
	}
	receiveApp = email.NewEmailApp(config)
	idelApp = email.NewEmailApp(config)
	go receiveApp.StartEmailReceive()
	go idelApp.StartIdle(receiveApp.UpdateChan)
	receiveApp.UpdateChan <- ""
	status = "running"
	sendMessage("status", status)
	select {}
}

func stopEmailLoop() {
	if receiveApp != nil {
		receiveApp.StopLoop()
		receiveApp = nil
	}
	if idelApp != nil {
		idelApp.StopLoop()
		idelApp = nil
	}
}

func loadConfig() *model.ServiceConfig {
	config := model.ServiceConfig{
		ContentPatterns: make([]model.ServiceContentPattern, 0),
	}
	if !checkConfigExist() {
		return &config
	}
	bytes, err := ioutil.ReadFile("config.mtt")
	if err != nil {
		log.Println(err)
		return &config
	}
	jsonStr := utils.AesDecryptCBC(bytes, []byte(*encryptKey))
	log.Println(string(jsonStr))
	err = json.Unmarshal(jsonStr, &config)
	if err != nil {
		log.Println(err)
		return &config
	}
	return &config
}

func saveConfig(config *model.ServiceConfig) error {
	if checkConfigExist() {
		os.Remove(configFileName)
	}
	bytes, err := json.Marshal(config)
	if err != nil {
		log.Println(err)
		return err
	}
	encryptBytes := utils.AesEncryptCBC(bytes, []byte(*encryptKey))
	err = ioutil.WriteFile(configFileName, encryptBytes, 0666)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
}

func checkConfigExist() bool {
	_, err := os.Stat(configFileName)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

var live = flag.Bool("live", false, "use live mode")
var port = flag.String("port", "1323", "http port")
var password = flag.String("password", "ucommune", "password")
var encryptKey = flag.String("encryptKey", "TISISVIRGLCRATDP", "encrypt key length must be 16 strings")

func main() {
	flag.Parse()
	e := echo.New()
	log.Printf("flag set %v %v %v\n", *live, *port, *password)
	assetHandler := http.FileServer(getFileSystem(*live))
	e.GET("/", echo.WrapHandler(assetHandler))
	e.GET("/api/password/:passText", loginHandler)
	e.POST("/api/service/start", startServiceHandler)
	e.GET("/api/service/stop", stopServiceHandler)
	e.GET("/ws", webSocketHandler)
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", assetHandler)))
	e.Logger.Fatal(e.Start(":" + *port))
}
