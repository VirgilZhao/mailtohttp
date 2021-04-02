package main

import (
	"embed"
	"encoding/hex"
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
	if *password == passwordCheck {
		return c.JSON(200, "ok")
	}
	return c.JSON(200, "fail")
}

func startServiceHandler(c echo.Context) error {
	config := model.ServiceConfig{}
	if err := c.Bind(&config); err != nil {
		return c.JSON(200, err.Error())
	}
	go startEmailLoop(&config)
	saveConfig(&config)
	return c.JSON(200, "ok")
}

func stopServiceHandler(c echo.Context) error {
	return c.JSON(200, "ok")
}

var logSocket = websocket.Upgrader{}
var msgChan chan string = make(chan string)
var stopChan chan string = make(chan string)

func webSocketHandler(c echo.Context) error {
	ws, err := logSocket.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		panic(err)
	}
	defer ws.Close()
	for {
		select {
		case msg := <-msgChan:
			err := ws.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				log.Println(err)
			}
			break
		case <-stopChan:
			log.Println("stop socket")
			return nil
			break
		}
	}
	return nil
}

func startEmailLoop(config *model.ServiceConfig) {
	if receiveApp != nil {
		receiveApp.StopEmailReceive()
	}
	if idelApp != nil {
		idelApp.StopIdle()
	}
	receiveApp = email.NewEmailApp(config.EmailSettings.ImapAddress, config.EmailSettings.ImapPort, config.EmailSettings.Email, config.EmailSettings.Password, config.EmailSettings.Folder, &msgChan)
	idelApp = email.NewEmailApp(config.EmailSettings.ImapAddress, config.EmailSettings.ImapPort, config.EmailSettings.Email, config.EmailSettings.Password, config.EmailSettings.Folder, &msgChan)
	go receiveApp.StartEmailReceive()
	go idelApp.StartIdle(receiveApp.UpdateChan)
	select {}
}

func saveConfig(config *model.ServiceConfig) error {
	bytes, err := json.Marshal(config)
	if err != nil {
		log.Println(err)
		return err
	}
	encryptBytes := utils.AesEncryptCBC(bytes, []byte(*encryptKey))
	encryptStr := hex.EncodeToString(encryptBytes)
	err = ioutil.WriteFile("config.mtt", []byte(encryptStr), 0666)
	if err != nil {
		log.Println(err)
		return err
	}
	return nil
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
