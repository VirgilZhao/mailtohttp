package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"github.com/labstack/echo/v4"
)

//go:embed app
var embedFiles embed.FS

func getFileSystem(useOs bool)  http.FileSystem {
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

func main()  {
	e := echo.New()
	useOS := len(os.Args) > 1 && os.Args[1] == "live"
	assetHandler := http.FileServer(getFileSystem(useOS))
	e.GET("/", echo.WrapHandler(assetHandler))
	e.GET("/static/*", echo.WrapHandler(http.StripPrefix("/static/", assetHandler)))
	e.Logger.Fatal(e.Start(":1323"))
}