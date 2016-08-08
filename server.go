package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/bioothod/apparat/middleware"
	"github.com/bioothod/apparat/services/common"
	"github.com/gin-gonic/contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"github.com/reverbrain/warp/bindings/go/warp"
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
	IndexedId	string				`json:"indexed_id"`
	Author		string				`json:"author"`
	Title		string				`json:"title"`
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
	Completed	bool				`json:"completed"`
	NextDocumentId	string				`json:"next_document_id"`
	Docs		[]Document			`json:"ids"`
}

type Paging struct {
	MaxNumber	int64				`json:"max_number"`
	NextDocumentId	string				`json:"next_document_id"`
}

type TimeRange struct {
	Start		int64				`json:"start"`
	End		int64				`json:"end"`
}

type SearchRequest struct {
	Mbox		string				`json:"mailbox"`
	Exact		map[string]string		`json:"exact"`
	Negation	map[string]string		`json:"negation"`
	Query		map[string]string		`json:"query"`
	Time		TimeRange			`json:"time"`
	Paging		Paging				`json:"paging"`
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
	warp_addr := flag.String("warp", "", "warp lexical processing server")
	static_dir := flag.String("static", "", "directory for static content")

	flag.Parse()
	if *addr == "" {
		log.Fatalf("You must provide address where server will listen for incoming connections\n")
	}
	if *static_dir == "" {
		log.Fatalf("You must provide static content directory\n")
	}

	if *warp_addr == "" {
		log.Fatalf("You must provide warp server address:port\n")
	}
	if *greylock_addr == "" {
		log.Fatalf("You must provide greylock server address:port\n")
	}

	searcher, err := NewGreylockSearcher(*greylock_addr)
	if err != nil {
		log.Fatalf("Could not create greylock searcher: %v\n", err)
	}

	lp, err := warp.NewEngine(*warp_addr)
	if err != nil {
		log.Fatalf("Could not create warp lexical processor: %v\n", err)
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
		start_time := time.Now()

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

		qwords_stemmed := make([]string, 0)

		var mreq SearchRequest
		mreq.Mbox = req.Mbox
		mreq.Paging = req.Paging
		mreq.Time = req.Time
		mreq.Query = make(map[string]string)
		mreq.Exact = make(map[string]string)
		mreq.Negation = make(map[string]string)

		negation_prefix := "negation_"
		exact_prefix := "exact_"

		wr := warp.CreateRequest()
		for k, v := range req.Query {
			wr.Insert(k, v)
		}
		for k, v := range req.Exact {
			wr.Insert(exact_prefix + k, v)
		}
		for k, v := range req.Negation {
			wr.Insert(negation_prefix + k, v)
		}
		wr.WantStem = true
		wr.WantUrls = true

		wresp, err := lp.Convert(wr)
		if err != nil {
			estr := fmt.Sprintf("warp failed: %v", err)
			common.NewErrorString(c, "search", estr)

			c.JSON(http.StatusBadRequest, gin.H {
				"operation": "search",
				"error": estr,
			})
			return
		}

		for k, v := range wresp.Result {
			if k == "urls" {
				mreq.Query[k] = v.Text
				continue
			}

			if strings.HasPrefix(k, negation_prefix) {
				k = strings.TrimPrefix(k, negation_prefix)
				mreq.Negation[k] = v.Stem
			} else if strings.HasPrefix(k, exact_prefix) {
				k = strings.TrimPrefix(k, exact_prefix)
				mreq.Exact[k] = v.Text
			} else {
				mreq.Query[k] = v.Stem
			}

			for _, s := range strings.Split(v.Stem, " ") {
				if len(s) != 0 {
					qwords_stemmed = append(qwords_stemmed, s)
				}
			}
		}

		search_start_time := time.Now()
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
				high []int
			}
			last_indexed_end := -1
			off := 5

			indexes := make(map[int]*chunk)
			for idx, w := range content {
				for _, q := range qwords_stemmed {
					if !strings.HasPrefix(w, q) {
						continue
					}

					start := idx - off
					if start < 0 {
						start = 0
					}

					end := idx + off
					if end > len(content) {
						end = len(content)
					}

					if len(indexes) > 0 && start <= last_indexed_end {
						ch := indexes[len(indexes) - 1]
						ch.end = end
						ch.high = append(ch.high, idx)
					} else {
						ch := &chunk {
							start: start,
							end: end,
							high: []int{idx},
						}

						indexes[len(indexes)] = ch
					}

					last_indexed_end = end
				}
			}

			ret := make([]string, 0, len(indexes))
			for _, ch := range indexes {
				for idx := ch.start; idx < ch.end; idx++ {
					t := content[idx]
					for _, h := range ch.high {
						if idx == h {
							t = "<high>" + t + "</high>"
							break
						}
					}

					ret = append(ret, t)
				}

				ret = append(ret, "...")
			}

			return ret
		}

		postprocessing_start_time := time.Now()
		docs := make([]Document, 0, len(res.Docs))
		for _, doc := range res.Docs {
			doc.Title = strings.Join(doc.Content.Title, " ")
			doc.Content.Content = high(doc.Content.Content)
			doc.Content.Title = high(doc.Content.Title)

			docs = append(docs, doc)
		}

		res.Docs = docs

		completion_time := time.Now()

		clientIP := c.ClientIP()
		xreq := c.Request.Header.Get(middleware.XRequestHeader)

		glog.Infof("search: xreq: %s, client: %s, warp: request: %+v -> %+v, latencies: prepare: %s, search: %s, postprocessing: %s",
			xreq,
			clientIP,
			req, mreq,
			search_start_time.Sub(start_time).String(),
			postprocessing_start_time.Sub(search_start_time).String(),
			completion_time.Sub(postprocessing_start_time),
		)


		c.JSON(http.StatusOK, res)

	})

	http.ListenAndServe(*addr, r)
}
