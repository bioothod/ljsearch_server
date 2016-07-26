package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bioothod/apparat/middleware"
	"github.com/bioothod/apparat/services/common"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Page struct {
	Content		[]string		`json:"content"`
	Title		[]string		`json:"title"`
	Links		[]string		`json:"links"`
	Images		[]string		`json:"images"`
}

type Document struct {
	Id		string				`json:"id"`
	Author		string				`json:"author"`
	Content		Page				`json:"content"`
	Timestamp	time.Time			`json:"-"`
}

func (d *Document) MarshalJSON() ([]byte, error) {
	type Alias Document
	type Timestamp struct {
		Tsec		int64		`json:"tsec"`
		Tnsec		int64		`json:"tnsec"`
	}
	return json.Marshal(&struct {
		Timestamp Timestamp		`json:"timestamp"`
		*Alias
	}{
		Timestamp: Timestamp {
			Tsec:		d.Timestamp.Unix(),
			Tnsec:		0,
		},
		Alias: (*Alias)(d),
	})
}

func (d *Document) UnmarshalJSON(data []byte) (err error) {
	type Alias Document
	type Timestamp struct {
		Tsec		int64		`json:"tsec"`
		Tnsec		int64		`json:"tnsec"`
	}
	tmp := &struct {
		Timestamp	Timestamp	`json:"timestamp"`
		*Alias
	} {
		Alias: (*Alias)(d),
	}

	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	d.Timestamp = time.Unix(tmp.Timestamp.Tsec, tmp.Timestamp.Tnsec)
	return nil
}

type SearchResults struct {
	Mbox		string				`json:"mailbox"`
	Docs		[]Document			`json:"ids"`
}

type SearchRequest struct {
	Mbox		string				`json:"mailbox"`
	Query		map[string]string		`json:"query"`
}

type Searcher interface {
	Search(req *SearchRequest) (*SearchResults, error)
	Close()
}

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
	greylock_addr := flag.String("greylock", "", "greylock searching server")
	static_dir := flag.String("static", "", "directory for static content")

	flag.Parse()
	if *addr == "" {
		log.Fatalf("You must provide address where server will listen for incoming connections\n")
	}
	if *static_dir == "" {
		log.Fatalf("You must provide static content directory\n")
	}

	if *greylock_addr == "" {
		log.Fatalf("You must provide greylock server address:port\n")
	}

	searcher, err := NewGreylockSearcher(*greylock_addr)
	if err != nil {
		log.Fatalf("Could not create greylock searcher: %v\n", err)
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

	r.POST("/search", func (c *gin.Context) {
		var req SearchRequest
		err := c.BindJSON(&req)
		if err != nil {
			estr := fmt.Sprintf("cound not parse search request: %v", err)
			common.NewErrorString(c, "search", estr)

			c.JSON(http.StatusBadRequest, gin.H {
				"operation": "search",
				"error": estr,
			})
			return
		}

		var mreq SearchRequest
		mreq.Mbox = req.Mbox
		mreq.Query = make(map[string]string)

		qwords_stemmed := make([]string, 0)
		for k, v := range req.Query {
			reader := strings.NewReader(v)
			content, err := Parse(reader)
			if err != nil {
				estr := fmt.Sprintf("cound not parse search request query: '%s', error: %v", v, err)
				common.NewErrorString(c, "search", estr)

				c.JSON(http.StatusBadRequest, gin.H {
					"operation": "search",
					"error": estr,
				})
				return
			}

			mreq.Query[k] = strings.Join(content.StemmedText, " ")
			qwords_stemmed = append(qwords_stemmed, content.StemmedText...)
		}

		res, err := searcher.Search(&mreq)
		if err != nil {
			estr := fmt.Sprintf("search failed: %v", err)
			common.NewErrorString(c, "search", estr)

			c.JSON(http.StatusInternalServerError, gin.H {
				"operation": "search",
				"error": estr,
			})
			return
		}

		high := func(content []string) ([]string) {
			type chunk struct {
				start, end int
			}
			last_indexed_end := -1
			off := 5

			indexes := make(map[int]*chunk)
			for idx, w := range content {
				stemmed := Stem(w)
				for _, q := range qwords_stemmed {
					if stemmed == q {
						start := idx - off
						if start < 0 {
							start = 0
						}

						end := idx + off
						if end > len(content) {
							end = len(content)
						}

						ch := &chunk {
							start: start,
							end: end,
						}

						if len(indexes) > 0 && start <= last_indexed_end {
							indexes[len(indexes) - 1].end = end
						} else {
							indexes[len(indexes)] = ch
						}

						last_indexed_end = end
					}
				}
			}

			ret := make([]string, 0, len(indexes))
			for _, ch := range indexes {
				ret = append(ret, content[ch.start : ch.end]...)
				ret = append(ret, "...")
			}

			return ret
		}

		docs := make([]Document, 0, len(res.Docs))
		for _, doc := range res.Docs {
			doc.Content.Content = high(doc.Content.Content)
			doc.Content.Title = high(doc.Content.Title)

			docs = append(docs, doc)
		}

		res.Docs = docs

		c.JSON(http.StatusOK, res)

	})

	http.ListenAndServe(*addr, r)
}
