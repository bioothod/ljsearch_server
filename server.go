package main

import (
	"flag"
	"github.com/bioothod/apparat/middleware"
	"github.com/bioothod/apparat/services/common"
	"github.com/bioothod/apparat/services/aggregator"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"time"
)

func static_index_handler(root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file, err := os.Open(root + "/index.html")
		if err != nil {
			common.NewError(c, "static", err)

			c.Status(http.StatusBadRequest)
			return
		}
		defer file.Close()

		var t time.Time
		http.ServeContent(c.Writer, c.Request, "index.html", t, file)
	}
}


func main() {
	addr := flag.String("addr", "", "address to listen auth server at")
	search_addr := flag.String("search_addr", "", "address where searching greylock server listens")
	static_dir := flag.String("static", "", "directory for static content")

	flag.Parse()
	if *addr == "" {
		log.Fatalf("You must provide address where server will listen for incoming connections")
	}
	if *search_addr == "" {
		log.Fatalf("You must provide search server address")
	}
	if *static_dir == "" {
		log.Fatalf("You must provide static content directory")
	}


	r := gin.New()
	r.Use(middleware.XTrace())
	r.Use(middleware.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())


	// this is needed since otherwise ServeFile() redirects /index.html to / and there is no wildcard / handler
	// / wildcard handler can not be added, since it will clash with /get and other GET handlers
	// instead we have this static middleware which checks everything against static root and handles
	// files via http.FileServer.ServerHTTP() which ends up calling http.ServeFile() with its weird redirect
	r.GET("/index.html", static_index_handler(*static_dir))
	r.GET("/", static_index_handler(*static_dir))
	r.Use(static.Serve("/", static.LocalFile(*static_dir, false)))

	search_forwarder := &aggregator.Forwarder {
		Addr:	*search_addr,
	}

	r.POST("/search", func (c *gin.Context) {
		search_forwarder.Forward(c)
	})

	http.ListenAndServe(*addr, r)
}
